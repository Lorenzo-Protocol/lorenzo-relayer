package sender

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/jinzhu/copier"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/config"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
)

type Sender struct {
	chainfee.Estimator
	btcclient.BTCWallet
	vaultAddress btcutil.Address
	logger       *zap.SugaredLogger
}

func New(
	wallet btcclient.BTCWallet,
	vaultAddress btcutil.Address,
	est chainfee.Estimator,
	parentLogger *zap.Logger,
) *Sender {
	return &Sender{
		Estimator:    est,
		BTCWallet:    wallet,
		vaultAddress: vaultAddress,
		logger:       parentLogger.With(zap.String("module", "relayer")).Sugar(),
	}
}

func (s *Sender) SendBTCtoLorenzoVault(targetAddressOnLorenzoChain []byte, amount btcutil.Amount) error {
	if len(targetAddressOnLorenzoChain) != 20 {
		return fmt.Errorf("invalid target address on lorenzo chain , must be a valid EVM address")
	}

	utxo, err := s.PickOneUTXO(amount)
	if err != nil {
		return err
	}
	// recipient is a change address that all the
	// remaining balance of the utxo is sent to
	tx1, err := s.buildTxWithData(
		utxo,
		s.vaultAddress.ScriptAddress(),
		amount,
		targetAddressOnLorenzoChain,
	)
	if err != nil {
		return fmt.Errorf("failed to add data to tx: %w", err)
	}

	tx1.TxId, err = s.sendTxToBTC(tx1.Tx)
	if err != nil {
		return fmt.Errorf("failed to send tx1 to BTC: %w", err)
	}
	return nil
}

// calcMinRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
// Adapted from https://github.com/btcsuite/btcd/blob/f9cbff0d819c951d20b85714cf34d7f7cc0a44b7/mempool/policy.go#L61
func (rl *Sender) calcMinRelayFee(txVirtualSize int64) btcutil.Amount {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).
	minRelayFeeRate := rl.RelayFeePerKW().FeePerKVByte()

	rl.logger.Debugf("current minimum relay fee rate is %v", minRelayFeeRate)

	minRelayFee := minRelayFeeRate.FeeForVSize(txVirtualSize)

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minRelayFee > btcutil.MaxSatoshi {
		minRelayFee = btcutil.MaxSatoshi
	}

	return minRelayFee
}

func (rl *Sender) dumpPrivKeyAndSignTx(tx *wire.MsgTx, utxo *types.UTXO) (*wire.MsgTx, error) {
	// get private key
	err := rl.WalletPassphrase(rl.GetWalletPass(), rl.GetWalletLockTime())
	if err != nil {
		return nil, err
	}
	wif, err := rl.DumpPrivKey(utxo.Addr)
	if err != nil {
		return nil, err
	}
	// add signature/witness depending on the type of the previous address
	// if not segwit, add signature; otherwise, add witness
	segwit, err := isSegWit(utxo.Addr)
	if err != nil {
		return nil, err
	}
	// add unlocking script into the input of the tx
	tx, err = completeTxIn(tx, segwit, wif.PrivKey, utxo)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// PickHighUTXO picks a UTXO that has the highest amount
func (rl *Sender) PickHighUTXO() (*types.UTXO, error) {
	// get the highest UTXO and UTXOs' sum in the list
	topUTXO, _, err := rl.BTCWallet.GetHighUTXOAndSum()
	if err != nil {
		return nil, err
	}
	utxo, err := types.NewUTXO(topUTXO, rl.GetNetParams())
	if err != nil {
		return nil, fmt.Errorf("failed to convert ListUnspentResult to UTXO: %w", err)
	}
	rl.logger.Debugf("pick utxo with id: %v, amount: %v, confirmations: %v", utxo.TxID, utxo.Amount, topUTXO.Confirmations)

	return utxo, nil
}

// PickHighUTXO picks a UTXO that has the highest amount
func (rl *Sender) PickOneUTXO(amount btcutil.Amount) (*types.UTXO, error) {
	// get the highest UTXO and UTXOs' sum in the list
	topUTXO, _, err := rl.BTCWallet.GetHighUTXOAndSum()
	if err != nil {
		return nil, err
	}
	// TODO: just get <amount> sats utxo
	utxo, err := types.NewUTXO(topUTXO, rl.GetNetParams())
	if err != nil {
		return nil, fmt.Errorf("failed to convert ListUnspentResult to UTXO: %w", err)
	}
	rl.logger.Debugf("pick utxo with id: %v, amount: %v, confirmations: %v", utxo.TxID, utxo.Amount, topUTXO.Confirmations)

	return utxo, nil
}

// buildTxWithData builds a tx with data inserted as OP_RETURN
// note that OP_RETURN is set as the first output of the tx (index 0)
// and the rest of the balance is sent to a new change address
// as the second output with index 1
func (rl *Sender) buildTxWithData(
	utxo *types.UTXO,
	pkScript []byte,
	value btcutil.Amount,
	data []byte,
) (*types.BtcTxInfo, error) {
	rl.logger.Debugf("Building a BTC tx using %v with data %x", utxo.TxID.String(), data)
	tx := wire.NewMsgTx(wire.TxVersion)

	outPoint := wire.NewOutPoint(utxo.TxID, utxo.Vout)
	txIn := wire.NewTxIn(outPoint, nil, nil)
	// Enable replace-by-fee
	// See https://river.com/learn/terms/r/replace-by-fee-rbf
	txIn.Sequence = math.MaxUint32 - 2
	tx.AddTxIn(txIn)

	// build txout for data
	builder := txscript.NewScriptBuilder()
	tx.AddTxOut(wire.NewTxOut(value, pkScript))
	dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, err
	}
	tx.AddTxOut(wire.NewTxOut(0, dataScript))

	// build txout for change
	changeAddr, err := rl.GetChangeAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get change address: %w", err)
	}
	rl.logger.Debugf("Got a change address %v", changeAddr.String())
	changeScript, err := txscript.PayToAddrScript(changeAddr)
	if err != nil {
		return nil, err
	}
	copiedTx := &wire.MsgTx{}
	err = copier.Copy(copiedTx, tx)
	if err != nil {
		return nil, err
	}
	txSize, err := calculateTxVirtualSize(copiedTx, utxo, changeScript)
	if err != nil {
		return nil, err
	}
	minRelayFee := rl.calcMinRelayFee(txSize)
	if utxo.Amount < minRelayFee {
		return nil, fmt.Errorf("the value of the utxo is not sufficient for relaying the tx. Require: %v. Have: %v", minRelayFee, utxo.Amount)
	}
	txFee := rl.getFeeRate().FeeForVSize(txSize)
	// ensuring the tx fee is not lower than the minimum relay fee
	if txFee < minRelayFee {
		txFee = minRelayFee
	}
	// ensuring the tx fee is not higher than the utxo value
	if utxo.Amount < txFee {
		return nil, fmt.Errorf("the value of the utxo is not sufficient for paying the calculated fee of the tx. Calculated: %v. Have: %v", txFee, utxo.Amount)
	}
	change := utxo.Amount - txFee
	tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))

	// sign tx
	tx, err = rl.dumpPrivKeyAndSignTx(tx, utxo)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx: %w", err)
	}

	// serialization
	var signedTxBytes bytes.Buffer
	err = tx.Serialize(&signedTxBytes)
	if err != nil {
		return nil, err
	}

	rl.logger.Debugf("Successfully composed a BTC tx with balance of input: %v, "+
		"tx fee: %v, output value: %v, tx size: %v, hex: %v",
		utxo.Amount, txFee, change, txSize, hex.EncodeToString(signedTxBytes.Bytes()))

	return &types.BtcTxInfo{
		Tx:            tx,
		Utxo:          utxo,
		ChangeAddress: changeAddr,
		Size:          txSize,
		Fee:           txFee,
	}, nil
}

// getFeeRate returns the estimated fee rate, ensuring it within [tx-fee-max, tx-fee-min]
func (rl *Sender) getFeeRate() chainfee.SatPerKVByte {
	fee, err := rl.EstimateFeePerKW(uint32(rl.GetBTCConfig().TargetBlockNum))
	if err != nil {
		defaultFee := rl.GetBTCConfig().DefaultFee
		rl.logger.Errorf("failed to estimate transaction fee. Using default fee %v: %s", defaultFee, err.Error())
		return defaultFee
	}

	feePerKVByte := fee.FeePerKVByte()

	rl.logger.Debugf("current tx fee rate is %v", feePerKVByte)

	cfg := rl.GetBTCConfig()
	if feePerKVByte > cfg.TxFeeMax {
		rl.logger.Debugf("current tx fee rate is higher than the maximum tx fee rate %v, using the max", cfg.TxFeeMax)
		feePerKVByte = cfg.TxFeeMax
	}
	if feePerKVByte < cfg.TxFeeMin {
		rl.logger.Debugf("current tx fee rate is lower than the minimum tx fee rate %v, using the min", cfg.TxFeeMin)
		feePerKVByte = cfg.TxFeeMin
	}

	return feePerKVByte
}

func (rl *Sender) sendTxToBTC(tx *wire.MsgTx) (*chainhash.Hash, error) {
	rl.logger.Debugf("Sending tx %v to BTC", tx.TxHash().String())
	ha, err := rl.SendRawTransaction(tx, true)
	if err != nil {
		return nil, err
	}
	rl.logger.Debugf("Successfully sent tx %v to BTC", tx.TxHash().String())
	return ha, nil
}
