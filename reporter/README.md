# reporter

This package implements the lrzrelayer reporter. The lrzrelayer reporter is responsible for

- syncing the latest BTC blocks with a BTC node
- extracting headers and checkpoints from BTC blocks
- forwarding headers to a Lorenzo node
- detecting and reporting inconsistency between BTC blockchain and Lorenzo BTCLightclient header chain
- detecting and reporting stalling attacks where a checkpoint is w-deep on BTC but Lorenzo hasn't included its k-deep proof

The code is adapted from https://github.com/btcsuite/btcwallet/tree/master/wallet.