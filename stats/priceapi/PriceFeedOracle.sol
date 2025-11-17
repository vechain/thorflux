// Copyright (c) 2025 The VeChainThor developers
// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity 0.8.20;

interface PriceFeedOracle {
    function getLatestValue(bytes32 id) external view returns (uint128 value, uint128 updatedAt);
}