package pos

//go:generate docker run -v ./:/sources ethereum/solc:0.8.19 --evm-version paris --via-ir --overwrite --optimize --optimize-runs 200 -o /sources/compiled --abi --bin /sources/GetValidators.sol
