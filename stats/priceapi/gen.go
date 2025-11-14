package priceapi

//go:generate docker run -v ./:/sources ghcr.io/argotorg/solc:0.8.20 --evm-version paris --via-ir --overwrite --optimize --optimize-runs 200 -o /sources/compiled --abi --bin /sources/PriceFeedOracle.sol
//go:generate rm -rf ./compiled/PriceFeedOracle.bin
