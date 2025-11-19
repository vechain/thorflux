package slots_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/pubsub"
	"github.com/vechain/thorflux/stats/pos"
	"github.com/vechain/thorflux/stats/slots"
	"github.com/vechain/thorflux/types"
	"github.com/vechain/thorflux/vetutil"
)

func TestCorrectSeedPoA(t *testing.T) {
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

func TestCorrectSeedPoS(t *testing.T) {
	client := thorclient.New("https://testnet.vechain.org")
	fc := thor.GetForkConfig(thor.MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127"))
	fetcher := pubsub.NewBlockFetcher(client, fc.HAYABUSA, fc.HAYABUSA+thor.HayabusaTP())

	boundarySeedBlock := uint32(23350759) // the last block in the old Seed
	blockPreviousSeed := uint32(23350758) // the block with the previous seed

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
	newSeedStakers, err := pos.FetchValidations(newSeedBlk.Block.ID, client)
	require.Nil(t, err, "failed to fetch authority nodes")

	oldSeedStakers, err := pos.FetchValidations(oldSeedBlk.Block.ID, client)
	require.Nil(t, err, "failed to fetch authority nodes")

	// Log differences between newSeedStakers and oldSeedStakers
	compareStakerSets(t, newSeedStakers, oldSeedStakers)

	t.Log("Total Score:",
		" OldSeedBlock:", oldSeedBlk.Block.TotalScore,
		" NewSeedBlock:", newSeedBlk.Block.TotalScore,
		" Difference:", newSeedBlk.Block.TotalScore-oldSeedBlk.Block.TotalScore)

	// Calculate the expected proposers for newSeedBlk
	expectedNewSeedProposers := slots.NextBlockProposersPoS(convertStakerToPosNodes(newSeedStakers), newSeedBlk.Seed, newSeedBlk.Block.Number-1, 200)
	t.Log("NewSeedBlock Signer:", newSeedBlk.Block.Signer.String())
	t.Log("Expected NewSeedBlock Proposer", expectedNewSeedProposers[0].Master)
	require.Equal(t, newSeedBlk.Block.Signer, expectedNewSeedProposers[0].Master)

	// Calculate the expected proposers for oldSeedBlk
	expectedOldSeedProposers := slots.NextBlockProposersPoS(convertStakerToPosNodes(oldSeedStakers), oldSeedBlk.Seed, oldSeedBlk.Block.Number-1, 200)
	t.Log("OldSeedBlock Signer:", oldSeedBlk.Block.Signer.String())
	t.Log("Expected OldSeedBlock Proposer", expectedOldSeedProposers[0].Master)
	require.Equal(t, oldSeedBlk.Block.Signer, expectedOldSeedProposers[0].Master)

	// Calculate the expected proposers for newSeedBlk FROM oldSeedBlk
	expectedNewSeedProposers = slots.NextBlockProposersPoS(convertStakerToPosNodes(oldSeedStakers), oldSeedBlk.FutureSeed, oldSeedBlk.Block.Number, 200)
	t.Log("NewSeedBlock Signer:", oldSeedBlk.Block.Signer.String())
	t.Log("Expected NewSeedBlock Proposer", expectedNewSeedProposers[0].Master)
	require.Equal(t, newSeedBlk.Block.Signer, expectedNewSeedProposers[0].Master)
}

// convertStakerToPosNodes converts StakerInformation to PosNodes for proposer calculation
func convertStakerToPosNodes(stakerInfo *types.StakerInformation) []slots.PosNode {
	if stakerInfo == nil {
		return []slots.PosNode{}
	}

	posNodes := make([]slots.PosNode, 0)
	for _, v := range stakerInfo.Validations {
		// Convert weight from *big.Int to uint64
		weight := uint64(0)
		if v.TotalLockedWeight != nil && v.TotalLockedWeight.Sign() > 0 {
			weight = vetutil.ScaleToVET(v.TotalLockedWeight)
		}

		if weight == 0 {
			continue // Skip validators with zero weight
		}

		posNode := slots.PosNode{
			Master:   v.Address, // Use validator address as master
			Endorsor: v.Endorser,
			Active:   v.Online,
			Weight:   weight,
		}

		posNodes = append(posNodes, posNode)
	}

	return posNodes
}

// compareStakerSets compares two StakerInformation sets and logs differences
func compareStakerSets(t *testing.T, newStakers, oldStakers *types.StakerInformation) {
	t.Log("Comparing staker sets:")
	t.Log("NewSeedStakers count:", len(newStakers.Validations))
	t.Log("OldSeedStakers count:", len(oldStakers.Validations))

	if len(newStakers.Validations) != len(oldStakers.Validations) {
		t.Log("Different number of validators!")
	}

	// Create maps for easier comparison
	newStakersMap := make(map[string]*types.Validation)
	oldStakersMap := make(map[string]*types.Validation)

	for _, v := range newStakers.Validations {
		newStakersMap[v.Address.String()] = v
	}

	for _, v := range oldStakers.Validations {
		oldStakersMap[v.Address.String()] = v
	}

	// Check for validators only in newSeedStakers
	for addr, v := range newStakersMap {
		if _, exists := oldStakersMap[addr]; !exists {
			t.Log("Validator only in newSeedStakers:", addr)
			logValidationDetails(t, "NEW", v)
		}
	}

	// Check for validators only in oldSeedStakers
	for addr, v := range oldStakersMap {
		if _, exists := newStakersMap[addr]; !exists {
			t.Log("Validator only in oldSeedStakers:", addr)
			logValidationDetails(t, "OLD", v)
		}
	}

	// Check for validators with different properties
	totalDifferences := 0
	validatorsWithChanges := 0

	for addr, newV := range newStakersMap {
		if oldV, exists := oldStakersMap[addr]; exists {
			differences := compareValidations(newV, oldV)
			if len(differences) > 0 {
				validatorsWithChanges++
				totalDifferences += len(differences)
				t.Log("=== DIFFERENCES FOR VALIDATOR", addr, "===")
				for _, diff := range differences {
					t.Log("  ", diff)
				}
				t.Log("")
			}
		}
	}

	// Summary
	uniqueToNew := len(newStakersMap) - len(oldStakersMap) + len(newStakersMap) - len(oldStakersMap)
	uniqueToOld := len(oldStakersMap) - len(newStakersMap) + len(oldStakersMap) - len(newStakersMap)
	if uniqueToNew < 0 {
		uniqueToNew = 0
	}
	if uniqueToOld < 0 {
		uniqueToOld = 0
	}

	// Count unique validators
	uniqueInNew := 0
	uniqueInOld := 0
	for addr := range newStakersMap {
		if _, exists := oldStakersMap[addr]; !exists {
			uniqueInNew++
		}
	}
	for addr := range oldStakersMap {
		if _, exists := newStakersMap[addr]; !exists {
			uniqueInOld++
		}
	}

	t.Log("=== COMPARISON SUMMARY ===")
	if len(newStakers.Validations) == len(oldStakers.Validations) &&
		uniqueInNew == 0 && uniqueInOld == 0 &&
		validatorsWithChanges == 0 {
		t.Log("RESULT: Staker sets are EQUAL")
	} else {
		t.Log("RESULT: Staker sets are DIFFERENT")
		if uniqueInNew > 0 {
			t.Log("  - Validators only in NEW set:", uniqueInNew)
		}
		if uniqueInOld > 0 {
			t.Log("  - Validators only in OLD set:", uniqueInOld)
		}
		if validatorsWithChanges > 0 {
			t.Log("  - Validators with changes:", validatorsWithChanges)
			t.Log("  - Total field differences:", totalDifferences)
		}
	}
}

// compareValidations compares two validations and returns a list of differences
func compareValidations(newV, oldV *types.Validation) []string {
	var differences []string

	// Basic validation info
	if newV.Endorser != oldV.Endorser {
		differences = append(differences, fmt.Sprintf("Endorser: %v → %v", oldV.Endorser, newV.Endorser))
	}
	if (newV.Beneficiary == nil) != (oldV.Beneficiary == nil) ||
		(newV.Beneficiary != nil && oldV.Beneficiary != nil && *newV.Beneficiary != *oldV.Beneficiary) {
		differences = append(differences, fmt.Sprintf("Beneficiary: %v → %v", oldV.Beneficiary, newV.Beneficiary))
	}
	if newV.Status != oldV.Status {
		differences = append(differences, fmt.Sprintf("Status: %v → %v", oldV.Status, newV.Status))
	}
	if newV.Online != oldV.Online {
		differences = append(differences, fmt.Sprintf("Online: %v → %v", oldV.Online, newV.Online))
	}

	// Period info
	if newV.Period != oldV.Period {
		differences = append(differences, fmt.Sprintf("Period: %v → %v", oldV.Period, newV.Period))
	}
	if newV.CompletedPeriods != oldV.CompletedPeriods {
		differences = append(differences, fmt.Sprintf("CompletedPeriods: %v → %v", oldV.CompletedPeriods, newV.CompletedPeriods))
	}
	if newV.StartBlock != oldV.StartBlock {
		differences = append(differences, fmt.Sprintf("StartBlock: %v → %v", oldV.StartBlock, newV.StartBlock))
	}

	// Block references
	if (newV.ExitBlock == nil) != (oldV.ExitBlock == nil) ||
		(newV.ExitBlock != nil && oldV.ExitBlock != nil && *newV.ExitBlock != *oldV.ExitBlock) {
		differences = append(differences, fmt.Sprintf("ExitBlock: %v → %v", oldV.ExitBlock, newV.ExitBlock))
	}
	if (newV.OfflineBlock == nil) != (oldV.OfflineBlock == nil) ||
		(newV.OfflineBlock != nil && oldV.OfflineBlock != nil && *newV.OfflineBlock != *oldV.OfflineBlock) {
		differences = append(differences, fmt.Sprintf("OfflineBlock: %v → %v", oldV.OfflineBlock, newV.OfflineBlock))
	}

	// Validator VET amounts
	if newV.LockedVET != oldV.LockedVET {
		differences = append(differences, fmt.Sprintf("LockedVET: %v → %v", oldV.LockedVET, newV.LockedVET))
	}
	if newV.PendingUnlockVET != oldV.PendingUnlockVET {
		differences = append(differences, fmt.Sprintf("PendingUnlockVET: %v → %v", oldV.PendingUnlockVET, newV.PendingUnlockVET))
	}
	if newV.QueuedVET != oldV.QueuedVET {
		differences = append(differences, fmt.Sprintf("QueuedVET: %v → %v", oldV.QueuedVET, newV.QueuedVET))
	}
	if newV.CooldownVET != oldV.CooldownVET {
		differences = append(differences, fmt.Sprintf("CooldownVET: %v → %v", oldV.CooldownVET, newV.CooldownVET))
	}
	if newV.WithdrawableVET != oldV.WithdrawableVET {
		differences = append(differences, fmt.Sprintf("WithdrawableVET: %v → %v", oldV.WithdrawableVET, newV.WithdrawableVET))
	}
	if newV.Weight != oldV.Weight {
		differences = append(differences, fmt.Sprintf("Weight: %v → %v", oldV.Weight, newV.Weight))
	}

	// Total amounts (validator + delegators) - these are *big.Int
	if newV.TotalLockedStake != nil && oldV.TotalLockedStake != nil && newV.TotalLockedStake.Cmp(oldV.TotalLockedStake) != 0 {
		differences = append(differences, fmt.Sprintf("TotalLockedStake: %v → %v", oldV.TotalLockedStake, newV.TotalLockedStake))
	}
	if newV.TotalLockedWeight != nil && oldV.TotalLockedWeight != nil && newV.TotalLockedWeight.Cmp(oldV.TotalLockedWeight) != 0 {
		differences = append(differences, fmt.Sprintf("TotalLockedWeight: %v → %v", oldV.TotalLockedWeight, newV.TotalLockedWeight))
	}
	if newV.TotalQueuedStake != nil && oldV.TotalQueuedStake != nil && newV.TotalQueuedStake.Cmp(oldV.TotalQueuedStake) != 0 {
		differences = append(differences, fmt.Sprintf("TotalQueuedStake: %v → %v", oldV.TotalQueuedStake, newV.TotalQueuedStake))
	}
	if newV.TotalExitingStake != nil && oldV.TotalExitingStake != nil && newV.TotalExitingStake.Cmp(oldV.TotalExitingStake) != 0 {
		differences = append(differences, fmt.Sprintf("TotalExitingStake: %v → %v", oldV.TotalExitingStake, newV.TotalExitingStake))
	}
	if newV.NextPeriodWeight != nil && oldV.NextPeriodWeight != nil && newV.NextPeriodWeight.Cmp(oldV.NextPeriodWeight) != 0 {
		differences = append(differences, fmt.Sprintf("NextPeriodWeight: %v → %v", oldV.NextPeriodWeight, newV.NextPeriodWeight))
	}

	// Delegator amounts
	if newV.DelegatorStake.Cmp(oldV.DelegatorStake) != 0 {
		differences = append(differences, fmt.Sprintf("DelegatorStake: %v → %v", oldV.DelegatorStake, newV.DelegatorStake))
	}
	if newV.DelegatorWeight.Cmp(oldV.DelegatorWeight) != 0 {
		differences = append(differences, fmt.Sprintf("DelegatorWeight: %v → %v", oldV.DelegatorWeight, newV.DelegatorWeight))
	}
	if newV.DelegatorQueuedStake.Cmp(oldV.DelegatorQueuedStake) != 0 {
		differences = append(differences, fmt.Sprintf("DelegatorQueuedStake: %v → %v", oldV.DelegatorQueuedStake, newV.DelegatorQueuedStake))
	}
	if newV.DelegatorQueuedWeight.Cmp(oldV.DelegatorQueuedWeight) != 0 {
		differences = append(differences, fmt.Sprintf("DelegatorQueuedWeight: %v → %v", oldV.DelegatorQueuedWeight, newV.DelegatorQueuedWeight))
	}

	return differences
}

// logValidationDetails logs all details of a validation
func logValidationDetails(t *testing.T, prefix string, v *types.Validation) {
	t.Log(prefix, "Address:", v.Address)
	t.Log(prefix, "Endorser:", v.Endorser)
	t.Log(prefix, "Beneficiary:", v.Beneficiary)
	t.Log(prefix, "Period:", v.Period)
	t.Log(prefix, "CompletedPeriods:", v.CompletedPeriods)
	t.Log(prefix, "Status:", v.Status)
	t.Log(prefix, "StartBlock:", v.StartBlock)
	t.Log(prefix, "ExitBlock:", v.ExitBlock)
	t.Log(prefix, "OfflineBlock:", v.OfflineBlock)
	t.Log(prefix, "LockedVET:", v.LockedVET)
	t.Log(prefix, "PendingUnlockVET:", v.PendingUnlockVET)
	t.Log(prefix, "QueuedVET:", v.QueuedVET)
	t.Log(prefix, "CooldownVET:", v.CooldownVET)
	t.Log(prefix, "WithdrawableVET:", v.WithdrawableVET)
	t.Log(prefix, "Weight:", v.Weight)
	t.Log(prefix, "TotalLockedStake:", v.TotalLockedStake)
	t.Log(prefix, "TotalLockedWeight:", v.TotalLockedWeight)
	t.Log(prefix, "TotalQueuedStake:", v.TotalQueuedStake)
	t.Log(prefix, "TotalExitingStake:", v.TotalExitingStake)
	t.Log(prefix, "NextPeriodWeight:", v.NextPeriodWeight)
	t.Log(prefix, "Online:", v.Online)
	t.Log(prefix, "DelegatorStake:", v.DelegatorStake)
	t.Log(prefix, "DelegatorWeight:", v.DelegatorWeight)
	t.Log(prefix, "DelegatorQueuedStake:", v.DelegatorQueuedStake)
	t.Log(prefix, "DelegatorQueuedWeight:", v.DelegatorQueuedWeight)
}
