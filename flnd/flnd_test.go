package flnd

import (
	"testing"

	"github.com/flokiorg/flnd"
	"github.com/flokiorg/flnd/lncfg"
	"github.com/flokiorg/flnd/signal"
	"github.com/flokiorg/go-flokicoin/chaincfg"
)

func TestMain(t *testing.T) {

	walletdir := "/u/flzpace/xgit/repos/flokiorg/twallet/test/t0"
	network := &chaincfg.TestNet3Params

	interceptor, err := signal.Intercept()
	if err != nil {
		t.Fatal(err)
	}

	conf := flnd.DefaultConfig()
	conf.LndDir = walletdir
	conf.Flokicoin.Node = "neutrino"
	conf.NeutrinoMode.ConnectPeers = append(conf.NeutrinoMode.ConnectPeers, "node.loki:35212")
	conf.DebugLevel = "debug"
	conf.ProtocolOptions = &lncfg.ProtocolOptions{}
	conf.Pprof = &lncfg.Pprof{}
	conf.LogConfig.Console.Disable = true
	switch network {
	case &chaincfg.MainNetParams:
		conf.Flokicoin.MainNet = true
	case &chaincfg.TestNet3Params:
		conf.Flokicoin.TestNet3 = true
	case &chaincfg.TestNet4Params:
		conf.Flokicoin.TestNet4 = true
	case &chaincfg.SimNetParams:
		conf.Flokicoin.SigNet = true
	case &chaincfg.RegressionNetParams:
		conf.Flokicoin.RegTest = true
	case &chaincfg.SigNetParams:
		conf.Flokicoin.SigNet = true
	}

	config, err := flnd.ValidateConfig(conf, interceptor, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	impl := config.ImplementationConfig(interceptor)
	flndStarted := make(chan struct{}, 1)
	errCh := make(chan error)

	go func() {
		if err := flnd.Main(config, flnd.ListenerCfg{}, impl, interceptor, flndStarted); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	select {
	case err := <-errCh:
		t.Fatal(err)
	case <-flndStarted:
		t.Logf("started")
	}

}
