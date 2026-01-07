package flnd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/flokiorg/flnd"
	"github.com/flokiorg/flnd/signal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
)

var (
	ErrDaemonNotRunning = errors.New("daemon is not running")
)

const (
	FlndEndpoint = "localhost:10005"

	maxGrpcRecvMsgSize = 50 * 1024 * 1024
	maxGrpcSendMsgSize = 20 * 1024 * 1024
)

type daemon struct {
	config      *flnd.Config
	interceptor signal.Interceptor

	conn *grpc.ClientConn

	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	mu     sync.Mutex
	wg     sync.WaitGroup
	client *Client
}

func newDaemon(pctx context.Context, config *flnd.Config, interceptor signal.Interceptor) (*daemon, error) {

	ctx, cancel := context.WithCancel(pctx)

	if interceptor.ShutdownChannel() == nil {
		cancel()
		return nil, fmt.Errorf("signal interceptor is required")
	}

	return &daemon{
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
		interceptor: interceptor,
	}, nil
}

func (d *daemon) start() (c *Client, err error) {

	d.config, err = flnd.ValidateConfig(*d.config, d.interceptor, nil, nil)
	if err != nil {
		err = fmt.Errorf("failed to load config: %v", err)
		return
	}

	impl := d.config.ImplementationConfig(d.interceptor)
	defer func() {
		if err != nil {
			d.stop()
		}
	}()

	if err = d.exec(impl); err != nil {
		return
	}

	var creds credentials.TransportCredentials
	creds, err = tlsCreds(d.config.TLSCertPath)
	if err != nil {
		return nil, err
	}

	if len(d.config.RPCListeners) == 0 {
		return nil, fmt.Errorf("unable to open rpc connection, rpc listener is empty")
	}

	d.conn, err = grpc.NewClient(d.config.RPCListeners[0].String(),
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGrpcRecvMsgSize),
			grpc.MaxCallSendMsgSize(maxGrpcSendMsgSize),
			grpc.UseCompressor(gzip.Name),
		), grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
			Backoff: backoff.Config{
				BaseDelay:  500 * time.Millisecond,
				Multiplier: 1.5,
				MaxDelay:   5 * time.Second,
			},
		}))
	if err != nil {
		return nil, err
	}

	d.client = NewClient(d.ctx, d.conn, d.config)
	c = d.client
	return
}

func (d *daemon) exec(impl *flnd.ImplementationCfg) error {

	errCh := make(chan error)
	flndStarted := make(chan struct{})

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("unable to run FLND daemon: %v", r)
				if d.client != nil {
					d.client.kill(err)
					return
				}

				errCh <- err
			}
		}()
		if err := flnd.Main(d.config, flnd.ListenerCfg{}, impl, d.interceptor, flndStarted); err != nil {
			if d.client != nil {
				d.client.kill(err)
				return
			}

			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-flndStarted:
		return nil
	}
}

func (d *daemon) waitForShutdown() {
	d.wg.Wait()
	<-d.interceptor.ShutdownChannel()
	d.closed = true
}

func (d *daemon) stop() {

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}

	if d.client != nil {
		d.client.close()
	}

	if d.conn != nil {
		d.conn.Close()
	}

	d.cancel()
	d.interceptor.RequestShutdown()
	<-d.interceptor.ShutdownChannel()

}

func tlsCreds(certPath string) (credentials.TransportCredentials, error) {
	pem, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(pem) {
		return nil, errors.New("failed to parse cert")
	}
	return credentials.NewClientTLSFromCert(cp, ""), nil
}
