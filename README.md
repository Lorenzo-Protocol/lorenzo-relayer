# lorenzo-relayer

Lorenzo-relayer program for Lorenzo. It starts its development based on [Babylon vigilante v0.8.0](https://github.com/babylonchain/vigilante/releases/tag/v0.8.0)

## Requirements

- Go 1.21
- Package [libzmq](https://github.com/zeromq/libzmq)

## Building

In order to build the lrzrelayer,
```shell
make build
```

## Run locally
CONFIG_DIR is the directory of config file
```sh
./build/lrzrelayer reporter --config $CONFIG_DIR/lrzrelayer.yml
```