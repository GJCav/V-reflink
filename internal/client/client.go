package client

import (
	"context"
	"net"
	"time"

	"github.com/mdlayher/vsock"

	"github.com/GJCav/V-reflink/internal/framing"
	"github.com/GJCav/V-reflink/internal/protocol"
)

type DialFunc func(contextID, port uint32) (net.Conn, error)

type Client struct {
	HostCID uint32
	Port    uint32
	Timeout time.Duration
	Dial    DialFunc
}

func New(hostCID, port uint32, timeout time.Duration) *Client {
	return &Client{
		HostCID: hostCID,
		Port:    port,
		Timeout: timeout,
		Dial:    defaultDial,
	}
}

func (c *Client) Do(ctx context.Context, req protocol.Request) (*protocol.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if c.Timeout > 0 {
		var cancel context.CancelFunc
		if _, ok := ctx.Deadline(); !ok {
			ctx, cancel = context.WithTimeout(ctx, c.Timeout)
			defer cancel()
		}
	}

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if c.Timeout > 0 {
		if err := conn.SetDeadline(time.Now().Add(c.Timeout)); err != nil {
			return nil, err
		}
	}

	if err := framing.Write(conn, req); err != nil {
		return nil, err
	}

	var resp protocol.Response
	if err := framing.Read(conn, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}

	dial := c.Dial
	if dial == nil {
		dial = defaultDial
	}

	resultCh := make(chan result, 1)
	go func() {
		conn, err := dial(c.HostCID, c.Port)
		resultCh <- result{conn: conn, err: err}
	}()

	select {
	case <-ctx.Done():
		go func() {
			res := <-resultCh
			if res.conn != nil {
				_ = res.conn.Close()
			}
		}()
		return nil, ctx.Err()
	case res := <-resultCh:
		return res.conn, res.err
	}
}

func defaultDial(contextID, port uint32) (net.Conn, error) {
	return vsock.Dial(contextID, port, nil)
}
