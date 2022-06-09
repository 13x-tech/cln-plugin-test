package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/neutrino"
	"github.com/niftynei/glightning/gbitcoin"
	"github.com/niftynei/glightning/glightning"
)

const MaxFeeMultiple uint64 = 10

var btc *gbitcoin.Bitcoin

func main() {

	glightning.NewStringOption("neutrino-datadir", "data dir for neutrino", "~/.neutrino")
	plugin := glightning.NewPlugin(onInit)

	err := plugin.Start(os.Stdin, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}

func onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {
	log.Printf("successfully init'd! %s\n", config.RpcFile)

	bb := glightning.NewBitcoinBackend(plugin)

	dataDir := options["neutrino-datadir"].GetValue().(string)
	netWork := options["network"].GetValue().(string)

	n, err := NewBackend(dataDir, netWork)
	if err != nil {
		log.Fatal(err)
	}

	bb.RegisterGetUtxOut(n.GetUtxOut)
	bb.RegisterGetChainInfo(n.GetChainInfo)
	bb.RegisterGetFeeRate(n.GetFeeRate)
	bb.RegisterSendRawTransaction(n.SendRawTx)
	bb.RegisterGetRawBlockByHeight(n.BlockByHeight)
	bb.RegisterEstimateFees(n.EstimateFees)

	// btc info is set via plugin 'options'
	// neutrinoDir, _ := plugin.GetOption("neutrino-datadir")
	// default startup
}

func NewBackend(dataDir, network string) (*Neutrino, error) {

	var params chaincfg.Params

	switch network {
	case "bitcoin":
		params = chaincfg.MainNetParams
	case "testnet":
		params = chaincfg.TestNet3Params
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	params.RelayNonStdTxs = true

	neutrinoDataDir := path.Join(dataDir, "data")
	if err := os.MkdirAll(neutrinoDataDir, 0777); err != nil {
		return nil, fmt.Errorf("Error in os.MkdirAll %v\n", err)
	}

	neutrinoDB := path.Join(neutrinoDataDir, "wallet")
	db, err := walletdb.Create("bdb", neutrinoDB, true, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("Error in walletdb.Create: %v\n", err)
	}

	chainService, err := neutrino.NewChainService(neutrino.Config{
		DataDir:     neutrinoDataDir,
		ChainParams: params,
		Database:    db,
	})

	if err != nil {
		return nil, fmt.Errorf("Error in neutrino.NewChainService: %v\n", err)
	}

	return &Neutrino{
		chainService: chainService,
	}, nil
}

type Neutrino struct {
	chainService *neutrino.ChainService
}

func (n *Neutrino) GetUtxOut(txid string, vout uint32) (string, string, error) {

	hash, err := chainhash.NewHashFromStr(txid)
	if err != nil {
		return "", "", fmt.Errorf("could not get utxo hash: %w", err)
	}

	filterInput := neutrino.InputWithScript{
		OutPoint: *wire.NewOutPoint(
			hash,
			vout,
		),
	}

	sreport, err := n.chainService.GetUtxo(
		neutrino.WatchInputs(filterInput),
	)
	if err != nil {
		return "", "", fmt.Errorf("could not get utxo: %w", err)
	}

	amt := sreport.Output.Value
	amtMSat := int(amt) * 1000

	log.Printf("called getutxo")
	return fmt.Sprintf("%d", amtMSat), fmt.Sprintf("%x", sreport.Output.PkScript), nil
}

func (n *Neutrino) GetChainInfo() (*glightning.Btc_ChainInfo, error) {
	log.Printf("called getchaininfo")
	return &glightning.Btc_ChainInfo{}, nil
}

func (n *Neutrino) GetFeeRate(blocks uint32, mode string) (uint64, error) {
	log.Printf("called getfeerate %d %s", blocks, mode)
	return 0, nil
}

func (n *Neutrino) EstimateFees() (*glightning.Btc_EstimatedFees, error) {
	log.Printf("called estimatefees")

	return &glightning.Btc_EstimatedFees{}, nil
}

func (n *Neutrino) SendRawTx(tx string) error {

	txBytes, err := hex.DecodeString(tx)
	if err != nil {
		return fmt.Errorf("could not hex decode tx: %w", err)
	}

	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return fmt.Errorf("could not deserialize tx: %w", err)
	}

	return nil
}

// return a blockhash, block, error
func (n *Neutrino) BlockByHeight(height uint32) (string, string, error) {
	log.Printf("called blockbyheight %d", height)
	hash, err := n.chainService.GetBlockHash(int64(height))
	if err != nil {
		return "", "", fmt.Errorf("could not get blockhash: %w", err)
	}

	block, err := n.chainService.GetBlock(*hash)
	if err != nil {
		return "", "", fmt.Errorf("could not get block: %w", err)
	}

	bByts, err := block.Bytes()
	if err != nil {
		return "", "", fmt.Errorf("could not get block bytes: %w", err)
	}

	return hash.String(), fmt.Sprintf("%x", bByts), nil
}
