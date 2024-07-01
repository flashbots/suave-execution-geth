// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

contract Example {
    event SomeEvent(uint256 value);

    uint public counter;

    function get(uint256 value) public {
        emit SomeEvent(value);
    }

    function increment() public {
        counter++;
    }
}
