package slots

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/thor"
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
	proposerCalculator  *ProposerCalculator
	futureProposerCount int
}

// NewWriter creates a new slots writer
func NewWriter(thor *thorclient.Client) *Writer {
	return &Writer{
		thor:                thor,
		authorityNodeList:   NewAuthorityNodeList(),
		proposerCalculator:  NewProposerCalculator(),
		futureProposerCount: DefaultFutureProposerCount,
	}
}

// SetFutureProposerCount sets how many future proposers to calculate
func (w *Writer) SetFutureProposerCount(count int) {
	w.futureProposerCount = count
}

// Write processes a block event and returns InfluxDB points for slots investigation
func (w *Writer) Write(event *types.Event) ([]*write.Point, error) {
	// Skip if Hayabusa is active
	if event.HayabusaStatus.Active {
		return nil, nil
	}

	// Skip genesis blocks
	if event.Prev == nil {
		return nil, nil
	}

	block := event.Block
	prev := event.Prev

	expectedBlockProposers, err := w.proposerCalculator.NextBlockProposers(
		w.authorityNodeList.nodes,
		event.Seed,
		prev.Number, // Use previous block number for seed calculation
		w.futureProposerCount,
	)
	if err != nil {
		return nil, err
	}

	if block.Number == 23224453 {
		fmt.Println("23224453")

		futureProposers, err := w.proposerCalculator.NextBlockProposers(
			w.authorityNodeList.nodes,
			event.Seed,
			prev.Number, // Use previous block number for seed calculation
			w.futureProposerCount,
		)
		if err != nil {
			fmt.Printf("Error calculating future proposers: %v\n", err)
		}
		fmt.Printf("future proposers: %v\n", futureProposers)
		fmt.Printf("future proposer #1: %s\n", futureProposers[0].Master)
		fmt.Printf("current block signer: %s\n", block.Signer)
		fmt.Printf("current block Number: %d\n", block.Number)
	}

	// Refresh authority node list if needed
	if len(expectedBlockProposers) == 0 ||
		expectedBlockProposers[0].Master != block.Signer ||
		w.authorityNodeList.ShouldRefresh(block) {
		if err = w.refreshAuthorityNodes(block.ID); err != nil {
			slog.Error("Failed to refresh authority nodes for slots investigation",
				"error", err,
				"block", block.Number)
			return nil, err
		}
		fmt.Printf("block proposers Curr: %v\n", len(w.authorityNodeList.GetActiveNodes()))
		fmt.Println("props2: ", w.authorityNodeList.GetActiveNodes())
	}

	// Get all authority nodes (we'll filter to active ones in the calculator)
	allNodes := w.authorityNodeList.nodes

	// Skip if we have no authority nodes
	if len(allNodes) == 0 {
		slog.Warn("No authority nodes available for slots calculation, skipping block", "block", block.Number)
		return []*write.Point{}, nil
	}

	// Calculate future proposers
	futureProposers, err := w.proposerCalculator.NextBlockProposers(
		allNodes,
		event.Seed,
		prev.Number, // Use previous block number for seed calculation
		w.futureProposerCount,
	)
	if err != nil {
		slog.Error("Failed to calculate future proposers",
			"error", err,
			"block", block.Number)
		return nil, err
	}

	// Create InfluxDB points
	points := make([]*write.Point, 0, len(futureProposers))
	activeNodeCount := w.authorityNodeList.GetActiveCount()

	if activeNodeCount == 0 {
		slog.Warn("Active node count is 0, this will result in empty dashboard data",
			"block", block.Number,
			"total_nodes", len(allNodes))
	}

	slog.Debug("Creating InfluxDB points",
		"block", block.Number,
		"active_node_count", activeNodeCount,
		"future_proposers", len(futureProposers))

	for _, proposer := range futureProposers {
		fieldData := map[string]interface{}{
			"authority_node":     proposer.Master.String(),
			"endorsor_node":      proposer.Endorsor.String(),
			"current_signer":     block.Signer.String(),
			"total_active_nodes": activeNodeCount,
		}

		// Debug log for the first point only to avoid spam
		if proposer.Position == 1 {
			slog.Debug("Writing point data",
				"block", block.Number,
				"position", proposer.Position,
				"authority_node", proposer.Master.String(),
				"endorsor_node", proposer.Endorsor.String(),
				"current_signer", block.Signer.String(),
				"total_active_nodes", activeNodeCount)
		}

		point := write.NewPoint(
			MeasurementName,
			map[string]string{
				"chain_tag":    event.ChainTag,
				"block_number": strconv.Itoa(int(block.Number)),
				"position":     strconv.Itoa(proposer.Position),
			},
			fieldData,
			time.Unix(int64(block.Timestamp), 0),
		)
		points = append(points, point)
	}

	slog.Debug("Generated slots investigation data",
		"block", block.Number,
		"future_proposers", len(futureProposers),
		"active_nodes", activeNodeCount)

	return points, nil
}

// refreshAuthorityNodes fetches the latest authority node list from the blockchain
func (w *Writer) refreshAuthorityNodes(blockID thor.Bytes32) error {
	slog.Info("Refreshing authority nodes for slots investigation", "revision", blockID.String())

	nodes, err := FetchAuthorityNodes(w.thor, blockID)
	if err != nil {
		slog.Error("Failed to fetch authority nodes", "error", err, "revision", blockID.String())
		return err
	}

	slog.Debug("Fetched authority nodes", "count", len(nodes), "revision", blockID.String())
	w.authorityNodeList.SetNodes(nodes, blockID)
	return nil
}
