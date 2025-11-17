package slots_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/pubsub"
	"github.com/vechain/thorflux/stats/slots"
)

func TestCorrectSeed(t *testing.T) {
	client := thorclient.New("https://mainnet.vechain.org")
	fetcher := pubsub.NewBlockFetcher(client, math.MaxUint32, math.MaxUint32)

	boundarySeedBlock := uint32(23284800) // the last block in the old Seed
	blockPreviousSeed := uint32(23284799) // the block with the previous seed

	// Fetch the boundarySeedBlock block
	// newSeedBlk.Seed <- has the new seed
	newSeedBlk, err := fetcher.FetchBlock(boundarySeedBlock)
	require.Nil(t, err, "failed to fetch block")

	// Fetch the blockPreviousSeed block
	// oldSeedBlk.Seed <- has the old seed
	oldSeedBlk, err := fetcher.FetchBlock(blockPreviousSeed)
	require.Nil(t, err, "failed to fetch block")

	require.NotEqual(t, oldSeedBlk, newSeedBlk)

	// auth nodes did not change
	newSeedAuthorities, err := slots.FetchAuthorityNodes(client, newSeedBlk.Block.ID)
	require.Nil(t, err, "failed to fetch authority nodes")

	oldSeedAuthorities, err := slots.FetchAuthorityNodes(client, oldSeedBlk.Block.ID)
	require.Nil(t, err, "failed to fetch authority nodes")

	for i := range newSeedAuthorities {
		require.Equal(t, oldSeedAuthorities[i].Master.String(), newSeedAuthorities[i].Master.String())
		require.Equal(t, oldSeedAuthorities[i].Endorsor.String(), newSeedAuthorities[i].Endorsor.String())
		require.Equal(t, oldSeedAuthorities[i].Active, newSeedAuthorities[i].Active)
	}
	require.Equal(t, oldSeedAuthorities, newSeedAuthorities)

	// Calculate the expected proposers for newSeedBlk
	expectedNewSeedProposers := slots.NextBlockProposersPoA(oldSeedAuthorities, newSeedBlk.Seed, newSeedBlk.Block.Number-1, 200)
	t.Log("NewSeedBlock Signer:", newSeedBlk.Block.Signer.String())
	t.Log("Expected NewSeedBlock Proposer", fmt.Sprintf("%+v", expectedNewSeedProposers[0]))
	require.Equal(t, newSeedBlk.Block.Signer, expectedNewSeedProposers[0].Master)

	// Calculate the expected proposers for oldSeedBlk
	expectedOldSeedProposers := slots.NextBlockProposersPoA(oldSeedAuthorities, oldSeedBlk.Seed, oldSeedBlk.Block.Number-1, 200)
	t.Log("OldSeedBlock Signer:", oldSeedBlk.Block.Signer.String())
	t.Log("Expected OldSeedBlock Proposer", fmt.Sprintf("%+v", expectedOldSeedProposers[0]))
	require.Equal(t, oldSeedBlk.Block.Signer, expectedOldSeedProposers[0].Master)

	// Calculate the expected proposers for newSeedBlk FROM oldSeedBlk
	expectedNewSeedProposers = slots.NextBlockProposersPoA(oldSeedAuthorities, oldSeedBlk.FutureSeed, oldSeedBlk.Block.Number, 200)
	t.Log("NewSeedBlock Signer:", oldSeedBlk.Block.Signer.String())
	t.Log("Expected NewSeedBlock Proposer", fmt.Sprintf("%+v", expectedNewSeedProposers[0]))
	require.Equal(t, newSeedBlk.Block.Signer, expectedNewSeedProposers[0].Master)
}
