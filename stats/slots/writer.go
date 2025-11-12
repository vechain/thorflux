package slots

import (
	"log/slog"
	"math/big"
	"strconv"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thorclient"

	"github.com/vechain/thorflux/types"
)

const (
	DefaultFutureProposerCount = 10
	MeasurementName            = "slots"
)

// Writer handles writing slots investigation data to InfluxDB
type Writer struct {
	thor                *thorclient.Client
	authorityNodeList   *AuthorityNodeList
	futureProposerCount int
}

// NewWriter creates a new slots writer
func NewWriter(thor *thorclient.Client) *Writer {
	return &Writer{
		thor:                thor,
		authorityNodeList:   NewAuthorityNodeList(),
		futureProposerCount: DefaultFutureProposerCount,
	}
}

// SetFutureProposerCount sets how many future proposers to calculate
func (w *Writer) SetFutureProposerCount(count int) {
	w.futureProposerCount = count
}

// Write processes a block event and returns InfluxDB points for slots investigation
func (w *Writer) Write(event *types.Event) ([]*write.Point, error) {
	// Skip genesis blocks
	if event.Prev == nil {
		return nil, nil
	}

	block := event.Block
	prev := event.Prev

	var futureProposers []FutureProposer
	var err error
	var activeNodeCount int

	if event.HayabusaStatus.Active {
		// PoS mode (Hayabusa)
		futureProposers, activeNodeCount, err = w.calculatePosProposers(event, prev)
	} else {
		// PoA mode
		futureProposers, activeNodeCount, err = w.calculatePoaProposers(event, prev, block)
	}

	if err != nil {
		slog.Error("Failed to calculate future proposers",
			"error", err,
			"block", block.Number,
			"posActive", event.HayabusaStatus.Active,
		)
		return nil, err
	}

	// Create InfluxDB points
	points := make([]*write.Point, 0, len(futureProposers))

	if activeNodeCount == 0 {
		slog.Warn("Active node count is 0, this will result in empty dashboard data",
			"block", block.Number)
	}

	for _, proposer := range futureProposers {
		// Calculate weight in millions: 25M VET for PoA, actual weight for PoS
		var weightMillions float64
		if event.HayabusaStatus.Active {
			// For PoS, try to find the actual weight from staker data
			weightMillions = 25.0 // Default to 25M if not found
			if event.Staker != nil {
				for _, v := range event.Staker.Validations {
					if v.Address == proposer.Master && v.TotalLockedWeight != nil {
						if v.TotalLockedWeight.IsUint64() {
							// Convert from wei to millions of VET (wei / 1e18 / 1e6 = wei / 1e24)
							weightWei := v.TotalLockedWeight.Uint64()
							weightMillions = float64(weightWei) / 1e24
						} else {
							// For very large weights, convert via big.Float
							weightFloat := new(big.Float).SetInt(v.TotalLockedWeight)
							divisor := new(big.Float).SetFloat64(1e24)
							result := new(big.Float).Quo(weightFloat, divisor)
							weightMillions, _ = result.Float64()
						}
						break
					}
				}
			}
		} else {
			// For PoA, all validators have equal weight of 25M VET
			weightMillions = 25.0
		}

		fieldData := map[string]interface{}{
			"authority_node":     proposer.Master.String(),
			"endorsor_node":      proposer.Endorsor.String(),
			"current_signer":     block.Signer.String(),
			"total_active_nodes": activeNodeCount,
			"weight":             weightMillions,
		}

		point := write.NewPoint(
			MeasurementName,
			map[string]string{
				"chain_tag":    event.ChainTag,
				"block_number": strconv.Itoa(int(block.Number)),
				"position":     strconv.Itoa(proposer.Position),
				"pos_active":   strconv.FormatBool(event.HayabusaStatus.Active),
			},
			fieldData,
			time.Unix(int64(block.Timestamp), 0),
		)
		points = append(points, point)
	}

	return points, nil
}

// refreshAuthorityNodes fetches the latest authority node list from the blockchain
func (w *Writer) refreshAuthorityNodes(block *api.JSONExpandedBlock, seed []byte, prev *api.JSONExpandedBlock) error {
	expectedBlockProposers, err := NextBlockProposers(
		w.authorityNodeList.nodes,
		seed,
		prev.Number, // Use previous block number for seed calculation
		w.futureProposerCount,
	)
	if err != nil {
		return err
	}

	// Refresh authority node list if needed
	if len(expectedBlockProposers) == 0 ||
		expectedBlockProposers[0].Master != block.Signer ||
		w.authorityNodeList.ShouldRefresh(block) {

		nodes, err := FetchAuthorityNodes(w.thor, block.ID)
		if err != nil {
			slog.Error("Failed to fetch authority nodes", "error", err, "revision", block.ID)
			return err
		}

		w.authorityNodeList.SetNodes(nodes)
	}

	return nil
}

// calculatePoaProposers handles PoA consensus proposer calculation
func (w *Writer) calculatePoaProposers(event *types.Event, prev *api.JSONExpandedBlock, block *api.JSONExpandedBlock) ([]FutureProposer, int, error) {
	// Refresh authority node list if needed
	if err := w.refreshAuthorityNodes(block, event.Seed, prev); err != nil {
		return nil, 0, err
	}

	allActiveNodes := w.authorityNodeList.GetActiveNodes()

	// Skip if we have no authority nodes
	if len(allActiveNodes) == 0 {
		return []FutureProposer{}, 0, nil
	}

	// Calculate future proposers using PoA algorithm
	futureProposers, err := NextBlockProposersPoA(
		allActiveNodes,
		event.Seed,
		prev.Number, // Use previous block number for seed calculation
		w.futureProposerCount,
	)
	if err != nil {
		return nil, 0, err
	}

	return futureProposers, w.authorityNodeList.GetActiveCount(), nil
}

// calculatePosProposers handles PoS consensus proposer calculation
func (w *Writer) calculatePosProposers(event *types.Event, prev *api.JSONExpandedBlock) ([]FutureProposer, int, error) {
	if event.Staker == nil {
		return []FutureProposer{}, 0, nil
	}

	// Convert staker validations to PosNodes
	posNodes := make([]PosNode, 0)
	activeCount := 0

	for _, v := range event.Staker.Validations {
		if v.Status != validation.StatusActive {
			continue
		}

		// Convert weight from *big.Int to uint64
		weight := uint64(0)
		if v.TotalLockedWeight != nil && v.TotalLockedWeight.Sign() > 0 {
			// Ensure weight fits in uint64
			if v.TotalLockedWeight.IsUint64() {
				weight = v.TotalLockedWeight.Uint64()
			} else {
				// Cap at max uint64 if too large
				weight = ^uint64(0) // max uint64
			}
		}

		if weight == 0 {
			continue // Skip validators with zero weight
		}

		posNode := PosNode{
			Master:   v.Address, // Use validator address as master
			Endorsor: v.Endorser,
			Active:   true, // Already filtered to active
			Weight:   weight,
		}

		posNodes = append(posNodes, posNode)
		activeCount++
	}

	if len(posNodes) == 0 {
		return []FutureProposer{}, 0, nil
	}

	// Calculate future proposers using PoS algorithm
	futureProposers, err := NextBlockProposersPoS(
		posNodes,
		event.Seed,
		prev.Number, // Use previous block number for seed calculation
		w.futureProposerCount,
	)
	if err != nil {
		return nil, 0, err
	}

	return futureProposers, activeCount, nil
}
