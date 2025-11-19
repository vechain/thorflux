package slots

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/types"
	"github.com/vechain/thorflux/vetutil"
)

const (
	DefaultFutureProposerCount = 10
	MeasurementName            = "slots"
)

// Writer handles writing slots investigation data to InfluxDB
type Writer struct {
	futureProposerCount int
}

// New creates a new slots writer
func New() *Writer {
	return &Writer{
		futureProposerCount: DefaultFutureProposerCount,
	}
}

// SetFutureProposerCount sets how many future proposers to calculate
func (w *Writer) SetFutureProposerCount(count int) {
	w.futureProposerCount = count
}

// Write processes a block event and returns InfluxDB points for slots investigation
func (w *Writer) Write(event *types.Event) []*write.Point {
	// Skip genesis blocks
	if event.Prev == nil {
		return nil
	}

	var futureProposers []FutureProposer
	var activeNodeCount int
	var expectedBlkSigner thor.Address

	validationMap := map[thor.Address]*types.Validation{}
	if event.HayabusaStatus.Active {
		// PoS mode (Hayabusa)
		futureProposers, expectedBlkSigner, activeNodeCount = w.calculatePosProposers(event)
		validationMap = event.Staker.ValidationMap()
	} else {
		// PoA mode
		futureProposers, expectedBlkSigner, activeNodeCount = w.calculatePoaProposers(event)
	}

	// Create InfluxDB points
	points := make([]*write.Point, 0, len(futureProposers))

	if activeNodeCount == 0 {
		slog.Warn("Active node count is 0, this will result in empty dashboard data",
			"block", block.Number)
	}

	for _, proposer := range futureProposers {
		// Calculate weight in millions: 25M VET for PoA, actual weight for PoS
		weightMillions := 25.0
		if event.HayabusaStatus.Active {
			// For PoS, try to find the actual weight from staker data
			if event.Staker != nil {
				if val, ok := validationMap[proposer.Master]; ok {
					weightMillions = float64(vetutil.ScaleToMillionVET(val.TotalLockedWeight))
				}
			}
		}

		fieldData := map[string]interface{}{
			"authority_node":        proposer.Master.String(),
			"endorsor_node":         proposer.Endorsor.String(),
			"current_signer":        event.Block.Signer.String(),
			"total_active_nodes":    activeNodeCount,
			"weight":                weightMillions,
			"expected_block_signer": expectedBlkSigner,
		}

		point := write.NewPoint(
			MeasurementName,
			map[string]string{
				"block_number": strconv.Itoa(int(event.Block.Number)), // high cardinality tag, required for instant query
				"position":     strconv.Itoa(proposer.Position),
				"pos_active":   strconv.FormatBool(event.HayabusaStatus.Active),
			},
			fieldData,
			time.Unix(int64(event.Block.Timestamp), 0),
		)
		points = append(points, point)
	}

	return points
}

// calculatePoaProposers handles PoA consensus proposer calculation
func (w *Writer) calculatePoaProposers(event *types.Event) ([]FutureProposer, thor.Address, int) {
	// based on the previous block, calculate the expected proposers of current block
	expectedBlockProposers := NextBlockProposersPoA(
		event.ParentAuthNodes,
		event.Seed,
		event.Block.Number-1, // Use the previous block number to calculate current block
		w.futureProposerCount,
	)

	allActiveNodes := event.ParentAuthNodes.GetActiveNodes()

	// Skip if we have no authority nodes
	if len(allActiveNodes) == 0 {
		return []FutureProposer{}, thor.Address{}, 0
	}

	// Calculate future proposers using PoA algorithm
	futureProposers := NextBlockProposersPoA(event.AuthNodes, event.FutureSeed, event.Block.Number, w.futureProposerCount)
	return futureProposers, expectedBlockProposers[0].Master, event.AuthNodes.GetActiveCount()
}

// calculatePosProposers handles PoS consensus proposer calculation
func (w *Writer) calculatePosProposers(event *types.Event) ([]FutureProposer, thor.Address, int) {
	if event.Staker == nil {
		return []FutureProposer{}, thor.Address{}, 0
	}

	// Calculate expected block proposers using parent staker data (who should have signed current block)
	var expectedBlockProposers []FutureProposer
	if event.ParentStaker != nil {
		parentPosNodes := convertStakerToPosNodes(event.ParentStaker)
		expectedBlockProposers = NextBlockProposersPoS(
			parentPosNodes,
			event.Seed,
			event.Block.Number-1, // Use previous block number for seed calculation
			1,                    // Only need the first proposer for expected signer
		)
	}

	// Convert current staker validations to PosNodes for future proposers
	posNodes := convertStakerToPosNodes(event.Staker)
	if len(posNodes) == 0 {
		return []FutureProposer{}, thor.Address{}, 0
	}

	// Calculate future proposers using PoS algorithm
	futureProposers := NextBlockProposersPoS(
		posNodes,
		event.FutureSeed,
		event.Block.Number,
		w.futureProposerCount,
	)

	// Get expected signer from expectedBlockProposers
	expectedSigner := thor.Address{}
	if len(expectedBlockProposers) > 0 {
		expectedSigner = expectedBlockProposers[0].Master
	}

	return futureProposers, expectedSigner, len(posNodes)
}

// convertStakerToPosNodes converts StakerInformation to PosNodes for proposer calculation
func convertStakerToPosNodes(stakerInfo *types.StakerInformation) []PosNode {
	if stakerInfo == nil {
		return []PosNode{}
	}

	posNodes := make([]PosNode, 0)
	for _, v := range stakerInfo.Validations {
		// Convert weight from *big.Int to uint64
		weight := uint64(0)
		if v.TotalLockedWeight != nil && v.TotalLockedWeight.Sign() > 0 {
			weight = vetutil.ScaleToVET(v.TotalLockedWeight)
		}

		if weight == 0 {
			continue // Skip validators with zero weight
		}

		posNode := PosNode{
			Master:   v.Address, // Use validator address as master
			Endorsor: v.Endorser,
			Active:   v.Status == validation.StatusActive,
			Weight:   weight,
		}

		posNodes = append(posNodes, posNode)
	}

	return posNodes
}
