package main

import (
	"os"

	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/niftynei/glightning/glightning"
)

const MaxFeeMultiple uint64 = 10

func main() {

	n := NewBackend()

	plugin := glightning.NewPlugin(n.onInit)

	bb := glightning.NewBitcoinBackend(plugin)
	bb.RegisterGetChainInfo(n.GetChainInfo)
	bb.RegisterGetUtxOut(n.GetUtxOut)
	bb.RegisterSendRawTransaction(n.SendRawTx)
	bb.RegisterGetRawBlockByHeight(n.BlockByHeight)
	bb.RegisterEstimateFees(n.EstimateFees)

	strOption := glightning.NewStringOption("neutrino-datadir", "data dir for neutrino", "~/.neutrino")
	if err := plugin.RegisterOption(strOption); err != nil {
		panic(err)
	}

	feeAPI := glightning.NewStringOption("esplora-fee-api", "esplora api endpoint for fee estimation", "blockstream.info")
	if err := plugin.RegisterOption(feeAPI); err != nil {
		panic(err)
	}

	err := plugin.Start(os.Stdin, os.Stdout)
	if err != nil {
		panic(err)
	}

}
