package pubsub

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/influxdb"
)

type ForkHandler struct {
	db     *influxdb.DB
	client *thorclient.Client
}

const ForkMeasurement = "forks"

func NewForkHandler(db *influxdb.DB, client *thorclient.Client) *ForkHandler {
	return &ForkHandler{
		db:     db,
		client: client,
	}
}

// Resolve finds the common ancestor of the best chain and side chain, writes the forked blocks to InfluxDB.
// Then it wipes all entries the DB after the finalized block
func (h *ForkHandler) Resolve(best, sideChain, finalized *api.JSONExpandedBlock) error {
	stop := time.Now().Add(time.Hour)
	start := time.Unix(int64(finalized.Timestamp), 0).Add(time.Second)

	// delete all entries after finalized block
	if err := h.db.Delete(start, stop, fmt.Sprintf("_measurement!=\"%s\"", ForkMeasurement)); err != nil {
		return errors.Wrap(err, "failed to delete points after finalized block")
	}

	forkedChain, err := h.getSideChain(best, sideChain, finalized)
	if err != nil {
		return err
	}

	h.writeForkedBlock(forkedChain)

	return nil
}

func (h *ForkHandler) getSideChain(best, side, finalized *api.JSONExpandedBlock) ([]*api.JSONExpandedBlock, error) {
	if side.Number > best.Number {
		return nil, fmt.Errorf("side chain block number %d is greater than best chain block number %d", side.Number, best.Number)
	}
	bestNum := best.Number
	sideNum := side.Number

	// reduce best block num until it matches the best side block num
	for bestNum > sideNum {
		bestNum--
	}

	sideChain := make([]*api.JSONExpandedBlock, 0)

	// fetch the block at the same height as the side chain
	bestChainBlock, err := h.client.ExpandedBlock(strconv.FormatUint(uint64(bestNum), 10))
	if err != nil {
		return nil, err
	}

	if bestChainBlock.ID == side.ID {
		slog.Warn("can't resolve side chain, both blocks are the same", "block-num", bestNum)
		return nil, nil
	}

	sideChain = append(sideChain, side)
	var ancestor *api.JSONExpandedBlock

	slog.Info("🔍 searching for common ancestor", "side-num", side.Number, "best-num", best.Number, "finalized-num", finalized.Number)

	for {
		bestChainBlock, err = h.client.ExpandedBlock(bestChainBlock.ParentID.String())
		if err != nil {
			return nil, err
		}
		prev := sideChain[len(sideChain)-1]
		sideChainBlock, err := h.client.ExpandedBlock(prev.ParentID.String())
		if err != nil {
			return nil, err
		}
		if sideChainBlock.ID == bestChainBlock.ID {
			ancestor = sideChainBlock
			break
		}
		if sideChainBlock.Number == finalized.Number {
			slog.Error("fatal error finding common ancestor, reached finalized block", "finalized", finalized.Number, "side-length", len(sideChain))
			return nil, fmt.Errorf("failed to find common ancestor, reached finalized block %d", finalized.Number)
		}
		sideChain = append(sideChain, sideChainBlock)
	}

	slog.Info("‼️🍴side chain resolved", "ancestor-num", ancestor.Number, "side-length", len(sideChain))

	return sideChain, nil
}

// writeForkedChain writes the forked block and returns its parent
func (h *ForkHandler) writeForkedBlock(blocks []*api.JSONExpandedBlock) {
	for i, b := range blocks {
		t := time.Unix(int64(b.Timestamp), 0)

		p := write.NewPoint(ForkMeasurement, map[string]string{
			"group":  blocks[0].ID.String(), // easily create distinct groups of side chains
			"signer": b.Signer.String(),
		}, map[string]any{
			"number":    b.Number,
			"parent_id": b.ParentID.String(),
			"id":        b.ID.String(),
			"score":     b.TotalScore,
			"length":    len(blocks),
			"index":     len(blocks) - i, // blocks are stored descending, but index should be ascending
		}, t)

		h.db.WritePoints([]*write.Point{p})
	}
}
