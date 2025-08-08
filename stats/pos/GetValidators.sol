// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./compiled/Staker.sol";

interface Energy {
    function totalSupply() external view returns (uint256);

    function totalBurned() external view returns (uint256);
}

contract GetValidators {
    Staker private constant STAKER = Staker(payable(0x00000000000000000000000000005374616B6572));
    Energy private constant ENERGY = Energy(0x0000000000000000000000000000456E65726779);

    // staker stats
    function stakerBalance() public view returns (uint256) {
        return getBalance(address(STAKER));
    }

    function totalStake() public view returns (uint256, uint256) {
        return STAKER.totalStake();
    }

    function queuedStake() public view returns (uint256, uint256) {
        return STAKER.queuedStake();
    }

    function getBalance(address account) private view returns (uint256) {
        return account.balance;
    }

    // VTHO Stats
    function totalSupply() public view returns (uint256) {
        return ENERGY.totalSupply();
    }

    function totalBurned() public view returns (uint256) {
        return ENERGY.totalBurned();
    }

    function getValidators() public view returns (
        address[] memory,  // masters
        address[] memory, // endorsors
        uint256[] memory, // stake
        uint256[] memory, // weight
        uint8[] memory, // status
        bool[] memory, // online
        uint32[] memory, // stakingPeriod
        uint32[] memory, // startBlock
        uint32[] memory, // exitBlock
        uint32[] memory, // completedPeriods
        uint256[] memory, // delegatorsStake
        uint256[] memory, // delegatorsWeight
        uint256[] memory // totalStake
    ) {
        address[1000] memory idBuffer;
        uint count = 0;

        // populate active
        address first = STAKER.firstActive();
        while (first != address(0)) {
            idBuffer[count] = first;
            first = STAKER.next(first);
            count++;
        }

        // populate queued
        address next = STAKER.firstQueued();
        while (next != address(0)) {
            idBuffer[count] = next;
            next = STAKER.next(next);
            count++;
        }

        // Allocate output arrays
        address[] memory masters = new address[](count);
        address[] memory endorsors = new address[](count);
        uint256[] memory stake = new uint256[](count);
        uint256[] memory weight = new uint256[](count);
        uint8[] memory status = new uint8[](count);
        bool[] memory online = new bool[](count);
        uint32[] memory stakingPeriod = new uint32[](count);
        uint32[] memory startBlock = new uint32[](count);
        uint32[] memory exitBlock = new uint32[](count);
        uint32[] memory completedPeriods = new uint32[](count);
        uint256[] memory totalStakeAmt = new uint256[](count);
        uint256[] memory delegatorsStake = new uint256[](count);
        uint256[] memory delegatorsWeight = new uint256[](count);

        for (uint i = 0; i < count; i++) {
            address validatorId = idBuffer[i];
            (
                address endorsor,
                uint256 stakeAmount,
                uint256 weightAmount,
            ) = STAKER.getValidatorStake(validatorId);

            (
                uint8 validatorStatus,
                bool isOnline
            ) = STAKER.getValidatorStatus(validatorId);

            (
                uint32 period,
                uint32 start,
                uint32 exit,
                uint32 compPeriods
            ) = STAKER.getValidatorPeriodDetails(validatorId);

            masters[i] = validatorId;
            endorsors[i] = endorsor;
            stake[i] = stakeAmount;
            weight[i] = weightAmount;
            status[i] = validatorStatus;
            online[i] = isOnline;
            stakingPeriod[i] = period;
            startBlock[i] = start;
            exitBlock[i] = exit;
            completedPeriods[i] = compPeriods;
            (uint256 lockedStake, , uint256 dStake, uint256 dWeight) = STAKER.getValidationTotals(validatorId);
            delegatorsStake[i] = dStake;
            delegatorsWeight[i] = dWeight;
            totalStakeAmt[i] = lockedStake;
        }

        return (
            masters,
            endorsors,
            stake,
            weight,
            status,
            online,
            stakingPeriod,
            startBlock,
            exitBlock,
            completedPeriods,
            delegatorsStake,
            delegatorsWeight,
            totalStakeAmt
        );
    }
}
