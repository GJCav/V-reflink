package server

import (
	"context"
	"net"
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
