package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GJCav/V-reflink/internal/client"
	"github.com/GJCav/V-reflink/internal/protocol"
)

func TestClientServerRoundTrip(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	srv := &Server{
		Listener: listener,
		Handler: func(_ context.Context, req protocol.Request, _ PeerInfo) protocol.Response {
			if req.Src != "src.txt" || req.Dst != "dst.txt" {
				return protocol.ResponseFromError(protocol.NewError(protocol.CodeEINVAL, "unexpected request payload"))
			}
			return protocol.SuccessResponse()
		},
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}

	go func() {
		done <- srv.Serve(ctx)
	}()

	cli := &client.Client{
		Timeout: time.Second,
		Dial: func(_, _ uint32) (net.Conn, error) {
			return net.Dial("tcp", listener.Addr().String())
		},
	}

	resp, err := cli.Do(context.Background(), protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "src.txt",
		Dst:     "dst.txt",
	})
	if err != nil {
		t.Fatalf("Client.Do() error = %v", err)
	}
	if !resp.OK {
		t.Fatalf("response = %#v, want OK", resp)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Server.Serve() error = %v", err)
	}
}

func TestServeReturnsNilOnCanceledContextWithClosedVsockAcceptError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	listener := newBlockingAcceptErrorListener(&net.OpError{
		Op:  "accept",
		Net: "vsock",
		Err: errors.New("use of closed network connection"),
	})
	srv := &Server{
		Listener: listener,
		Handler: func(_ context.Context, _ protocol.Request, _ PeerInfo) protocol.Response {
			return protocol.SuccessResponse()
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx)
	}()

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Server.Serve() error = %v, want nil", err)
	}
}

func TestServeReturnsAcceptErrorWhenShutdownErrorIsUnexpected(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	wantErr := errors.New("accept failed")
	listener := newBlockingAcceptErrorListener(wantErr)
	srv := &Server{
		Listener: listener,
		Handler: func(_ context.Context, _ protocol.Request, _ PeerInfo) protocol.Response {
			return protocol.SuccessResponse()
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx)
	}()

	cancel()
	if err := <-done; !errors.Is(err, wantErr) {
		t.Fatalf("Server.Serve() error = %v, want %v", err, wantErr)
	}
}

type blockingAcceptErrorListener struct {
	acceptErr error
	closed    chan struct{}
	once      sync.Once
}

func newBlockingAcceptErrorListener(acceptErr error) *blockingAcceptErrorListener {
	return &blockingAcceptErrorListener{
		acceptErr: acceptErr,
		closed:    make(chan struct{}),
	}
}

func (l *blockingAcceptErrorListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, l.acceptErr
}

func (l *blockingAcceptErrorListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *blockingAcceptErrorListener) Addr() net.Addr {
	return stubAddr("vsock")
}

type stubAddr string

func (a stubAddr) Network() string { return string(a) }

func (a stubAddr) String() string { return string(a) }
