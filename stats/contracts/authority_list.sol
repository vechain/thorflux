// Copyright (c) 2025 The VeChainThor developers
// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity 0.8.20;

/// @title IAuthority interface
interface IAuthority {
    function first() external view returns(address);
    function next(address _nodeMaster) external view returns(address);
    function get(address _nodeMaster) external view returns(bool listed, address endorsor, bytes32 identity, bool active);
}

/// @title ListAuthority provides a method to list all authority nodes efficiently
contract ListAuthority {

    struct AuthorityNode {
        address nodeMaster;
        address endorsor;
        bool active;
    }

    IAuthority public constant authority = IAuthority(0x0000000000000000000000417574686f72697479);

    /// @notice Returns all listed authority nodes and their information
    function list() external view returns (AuthorityNode[] memory) {
        // Start with a reasonable initial capacity
        AuthorityNode[] memory nodes = new AuthorityNode[](102);
        uint256 index = 0;

        address current = authority.first();

        while (current != address(0)) {
            (bool listed, address endorsor, , bool active) = authority.get(current);

            if (listed) {
                // Expand array if needed
                if (index == nodes.length) {
                    nodes = _expand(nodes);
                }

                nodes[index] = AuthorityNode({
                    nodeMaster: current,
                    endorsor: endorsor,
                    active: active
                });
                index++;
            }

            current = authority.next(current);
        }

        // Shrink array to actual size before returning
        AuthorityNode[] memory result = new AuthorityNode[](index);
        for (uint256 i = 0; i < index; i++) {
            result[i] = nodes[i];
        }

        return result;
    }

    /// @dev Expands a memory array by doubling its capacity
    function _expand(AuthorityNode[] memory arr) internal pure returns (AuthorityNode[] memory bigger) {
        uint256 newLength = arr.length == 0 ? 10 : arr.length * 2;
        bigger = new AuthorityNode[](newLength);
        for (uint256 i = 0; i < arr.length; i++) {
            bigger[i] = arr[i];
        }
    }
}
