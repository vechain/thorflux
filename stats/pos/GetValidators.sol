// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

interface Staker {
    function get(address id) external view returns (
        address, address, uint256, uint256, uint8, bool, uint32, uint32, uint32
    );
    function firstActive() external view returns (address);
    function next(address id) external view returns (address);
}

contract GetValidators {
    Staker private constant STAKER = Staker(0x00000000000000000000000000005374616B6572);

    function getAll() external view returns (
        address[] memory, address[] memory, address[] memory,
        uint256[] memory, uint256[] memory, uint8[] memory,
        bool[] memory, uint32[] memory, uint32[] memory, uint32[] memory
    ) {
        address[101] memory idBuffer;
        uint count = 0;

        address id = STAKER.firstActive();
        while (id != address (0) && count < 101) {
            idBuffer[count] = id;
            id = STAKER.next(id);
            count++;
        }

        // Allocate output arrays
        address[] memory ids = new address[](count);
        address[] memory masters = new address[](count);
        address[] memory endorsors = new address[](count);
        uint256[] memory stake = new uint256[](count);
        uint256[] memory weight = new uint256[](count);
        uint8[] memory status = new uint8[](count);
        bool[] memory online = new bool[](count);
        uint32[] memory stakingPeriod = new uint32[](count);
        uint32[] memory startBlock = new uint32[](count);
        uint32[] memory exitBlock = new uint32[](count);

        for (uint i = 0; i < count; i++) {
            address  validatorId = idBuffer[i];
            ids[i] = validatorId;

            (
                address master, address endorsor,
                uint256 stakeAmount, uint256 weightAmount,
                uint8 validatorStatus, bool isOnline,
                uint32 period, uint32 start, uint32 exit
            ) = STAKER.get(validatorId);

            masters[i] = master;
            endorsors[i] = endorsor;
            stake[i] = stakeAmount;
            weight[i] = weightAmount;
            status[i] = validatorStatus;
            online[i] = isOnline;
            stakingPeriod[i] = period;
            startBlock[i] = start;
            exitBlock[i] = exit;
        }

        return (
            ids, masters, endorsors, stake, weight, status, online, stakingPeriod, startBlock, exitBlock
        );
    }
}
