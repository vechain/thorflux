package contracts

//go:generate sh -c "docker run --rm -v .:/src ghcr.io/argotorg/solc:0.8.20 --evm-version paris --combined-json abi,bin,bin-runtime,hashes /src/authority_list.sol | docker run --rm -i -v .:/src otherview/solgen:latest --out /src/generated"
