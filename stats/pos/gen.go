package pos

//go:generate curl https://raw.githubusercontent.com/vechain/thor/refs/heads/release/hayabusa/builtin/gen/staker.sol -o ./compiled/Staker.sol
//go:generate docker run -v ./:/sources ghcr.io/argotorg/solc:0.8.20 --evm-version paris --via-ir --overwrite --optimize --optimize-runs 200 -o /sources/compiled --abi --bin /sources/GetValidators.sol
