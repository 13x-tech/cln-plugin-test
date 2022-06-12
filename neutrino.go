package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/neutrino"
	"github.com/niftynei/glightning/glightning"
)

type Neutrino struct {
	esploraAPI   string
	network      string
	dataDir      string
	chainService *neutrino.ChainService
}

func NewBackend() *Neutrino {
	return &Neutrino{}
}

func (n *Neutrino) Start() error {

	var params chaincfg.Params

	switch n.network {
	case "main":
		log.Printf("creating mainnet configs")
		params = chaincfg.MainNetParams
	case "test":
		log.Printf("creating testnet configs")
		params = chaincfg.TestNet3Params
	default:
		return fmt.Errorf("unsupported network")
	}

	params.RelayNonStdTxs = true

	neutrinoDB := path.Join(n.dataDir, "wallet")
	db, err := walletdb.Create("bdb", neutrinoDB, true, 60*time.Second)
	if err != nil {
		return fmt.Errorf("error creating wallet : %wn", err)
	}

	chainService, err := neutrino.NewChainService(neutrino.Config{
		DataDir:     n.dataDir,
		ChainParams: params,
		Database:    db,
	})

	if err != nil {
		return fmt.Errorf("error in creating neutrino chainservice: %w", err)
	}

	if err := chainService.Start(); err != nil {
		return fmt.Errorf("could not start chainservice: %w", err)
	}

	n.chainService = chainService
	n.waitForPeers()
	n.waitForSync()

	return nil
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

	bestBlock, err := n.chainService.BestBlock()
	if err != nil {
		return nil, fmt.Errorf("could not get best block: %w", err)
	}

	_, height, err := n.chainService.BlockHeaders.ChainTip()
	if err != nil {
		return nil, fmt.Errorf("could not get chaintip: %w", err)
	}

	log.Printf("HeaderCount: %d\n", height)
	log.Printf("BlockCount: %d\n", uint32(bestBlock.Height))
	log.Printf("IBD: %v\n", !n.chainService.IsCurrent())

	return &glightning.Btc_ChainInfo{
		Chain:                n.network,
		HeaderCount:          height,
		BlockCount:           uint32(bestBlock.Height),
		InitialBlockDownload: !n.chainService.IsCurrent(),
	}, nil
}

func (n *Neutrino) feeAPIEndpoint() string {
	switch n.network {
	case "main":
		return fmt.Sprintf("https://%s/api", n.esploraAPI)
	case "test":
		return fmt.Sprintf("https://%s/testnet/api", n.esploraAPI)
	}
	return ""
}

func (n *Neutrino) EstimateFees() (*glightning.Btc_EstimatedFees, error) {
	log.Printf("called estimatefees")

	feeAPIURL := fmt.Sprintf("%s/fee-estimates", n.feeAPIEndpoint())

	results, err := http.Get(feeAPIURL)
	if err != nil {
		return nil, fmt.Errorf("could not fetch fee API: %w", err)
	}

	resultBytes, err := ioutil.ReadAll(results.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read API results: %w", err)
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal(resultBytes, &resultMap); err != nil {
		return nil, fmt.Errorf("invalid result json format: %w", err)
	}

	//Multiply by 1000 for kVByte instead of vByte
	slow := resultMap["144"].(float64) * 1000
	normal := resultMap["5"].(float64) * 1000
	urgent := resultMap["3"].(float64) * 1000
	veryUrgent := resultMap["2"].(float64) * 1000

	return &glightning.Btc_EstimatedFees{
		Opening:         uint64(normal),
		MutualClose:     uint64(normal),
		UnilateralClose: uint64(veryUrgent),
		DelayedToUs:     uint64(normal),
		HtlcResolution:  uint64(urgent),
		Penalty:         uint64(urgent),
		MinAcceptable:   uint64(slow),
		MaxAcceptable:   uint64(veryUrgent),
	}, nil
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

func (n *Neutrino) BlockByHeight(height uint32) (string, string, error) {
	log.Printf("called blockbyheight %d", height)
	hash, err := n.chainService.GetBlockHash(int64(height))
	if err != nil {
		return "", "", fmt.Errorf("could not get block: %w", err)
	}

	block, err := n.chainService.GetBlock(*hash)
	if err != nil {
		return "", "", fmt.Errorf("could not get block: %w", err)
	}

	bByts, err := block.Bytes()
	if err != nil {
		return "", "", fmt.Errorf("could not get block bytes: %w", err)
	}

	blockHex := hex.EncodeToString(bByts)

	return hash.String(), blockHex, nil
}

func (n *Neutrino) setDataDir(dataDir string) error {
	n.dataDir = dataDir
	neutrinoDataDir := path.Join(dataDir, "data")
	if err := os.MkdirAll(neutrinoDataDir, 0777); err != nil {
		return fmt.Errorf("error making directory %s: %w", dataDir, err)
	}
	return nil
}

func (n *Neutrino) setFeeAPI(api string) error {
	if api == "" {
		return fmt.Errorf("must include Esplora fee API")
	}

	n.esploraAPI = api
	return nil
}

func (n *Neutrino) setNetwork(network string) error {
	var net string

	switch network {
	case "bitcoin":
		net = "main"
	case "testnet":
		net = "test"
	default:
		return fmt.Errorf("unsupported network: %s", network)
	}

	n.network = net
	return nil
}

func (n *Neutrino) onInit(plugin *glightning.Plugin, options map[string]glightning.Option, config *glightning.Config) {

	feeAPI, err := plugin.GetOption("esplora-fee-api")
	if err != nil {
		panic(err)
	}

	if err := n.setFeeAPI(feeAPI); err != nil {
		panic(err)
	}

	if err := n.setNetwork(config.Network); err != nil {
		panic(err)
	}

	dataDir, err := plugin.GetOption("neutrino-datadir")
	if err != nil {
		panic(err)
	}

	if err := n.setDataDir(dataDir); err != nil {
		panic(err)
	}

	if err := n.Start(); err != nil {
		panic(err)
	}

	log.Printf("successfully init'd! %s\n", config.RpcFile)
}

func (w *Neutrino) waitForPeers() {
	log.Printf("Neutrino Loading\n")
	connected := checkPeers(w.chainService)
	log.Printf("Finding peers...\n")
	for {
		<-connected
		log.Printf("Connected!\n")
		break
	}
}

func (w *Neutrino) waitForSync() {
	synced := testCurrent(w.chainService)
	log.Printf("Syncing chain headers...\n")
	<-synced
	log.Printf("Done Syncing!\n")
}

func testCurrent(srv *neutrino.ChainService) chan struct{} {
	synced := make(chan struct{}, 1)
	go func() {
		for {
			if !srv.IsCurrent() {
				tip, height, _ := srv.BlockHeaders.ChainTip()
				log.Printf("still syncing...: %d - %s\n", height, tip.Timestamp)
				<-time.After(5 * time.Second)
				continue
			}
			break
		}
		synced <- struct{}{}
	}()
	return synced
}

func checkPeers(srv *neutrino.ChainService) chan struct{} {
	connected := make(chan struct{}, 1)
	go func() {
		count := 0
		for {
			if count > 60 {
				log.Fatal("could not find peers after 60 seconds")
			}
			if srv.ConnectedCount() < 1 {
				count++
				time.Sleep(1 * time.Second)
				continue
			}
			break
		}
		connected <- struct{}{}
	}()
	return connected
}
