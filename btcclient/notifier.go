package btcclient

import (
	"time"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
	"github.com/lightningnetwork/lnd/chainntnfs"
)

type Btcd struct {
	RPCHost        string
	RPCUser        string
	RPCPass        string
	RPCCert        string
	RawCert        string
	DisableTLS     bool
	BlockCacheSize uint64
}

type Bitcoind struct {
	RPCHost              string
	RPCUser              string
	RPCPass              string
	ZMQPubRawBlock       string
	ZMQPubRawTx          string
	ZMQReadDeadline      time.Duration
	EstimateMode         string
	PrunedNodeMaxPeers   int
	RPCPolling           bool
	BlockPollingInterval time.Duration
	TxPollingInterval    time.Duration
	BlockCacheSize       uint64
}

type BtcNodeBackendConfig struct {
	Btcd              *Btcd
	Bitcoind          *Bitcoind
	ActiveNodeBackend types.SupportedBtcBackend
}

type NodeBackend struct {
	chainntnfs.ChainNotifier
}

type HintCache interface {
	chainntnfs.SpendHintCache
	chainntnfs.ConfirmHintCache
}

// type for disabled hint cache
// TODO: Determine if we need hint cache backed up by database which is provided
// by lnd.
type EmptyHintCache struct{}

var _ HintCache = (*EmptyHintCache)(nil)

func (c *EmptyHintCache) CommitSpendHint(height uint32, spendRequests ...chainntnfs.SpendRequest) error {
	return nil
}
func (c *EmptyHintCache) QuerySpendHint(spendRequest chainntnfs.SpendRequest) (uint32, error) {
	return 0, nil
}
func (c *EmptyHintCache) PurgeSpendHint(spendRequests ...chainntnfs.SpendRequest) error {
	return nil
}

func (c *EmptyHintCache) CommitConfirmHint(height uint32, confRequests ...chainntnfs.ConfRequest) error {
	return nil
}
func (c *EmptyHintCache) QueryConfirmHint(confRequest chainntnfs.ConfRequest) (uint32, error) {
	return 0, nil
}
func (c *EmptyHintCache) PurgeConfirmHint(confRequests ...chainntnfs.ConfRequest) error {
	return nil
}
