package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/vsock"

	"github.com/GJCav/V-reflink/internal/framing"
	"github.com/GJCav/V-reflink/internal/protocol"
)

type PeerInfo struct {
	CID uint32
}

type HandlerFunc func(context.Context, protocol.Request, PeerInfo) protocol.Response

type Server struct {
	Listener     net.Listener
	Handler      HandlerFunc
	Logger       *slog.Logger
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (s *Server) Serve(ctx context.Context) error {
	if s.Listener == nil {
		return errors.New("listener is required")
	}
	if s.Handler == nil {
		return errors.New("handler is required")
	}

	go func() {
		<-ctx.Done()
		_ = s.Listener.Close()
	}()

	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			if ctx.Err() != nil && isExpectedShutdownAcceptError(err) {
				return nil
			}
			return err
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	peer := peerFromAddr(conn.RemoteAddr())

	if s.ReadTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(s.ReadTimeout))
	}

	var req protocol.Request
	if err := framing.Read(conn, &req); err != nil {
		s.logger().Error("failed to read request", "cid", peer.CID, "err", err)
		_ = s.writeResponse(conn, protocol.ResponseFromError(protocol.WrapError(protocol.CodeEINVAL, "invalid request", err)))
		return
	}

	_ = conn.SetReadDeadline(time.Time{})

	resp := s.Handler(context.Background(), req, peer)
	if err := s.writeResponse(conn, resp); err != nil {
		s.logger().Error("failed to write response", "cid", peer.CID, "err", err)
	}
}

func (s *Server) writeResponse(conn net.Conn, resp protocol.Response) error {
	if s.WriteTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
		defer conn.SetWriteDeadline(time.Time{})
	}

	return framing.Write(conn, resp)
}

func (s *Server) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}

	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func peerFromAddr(addr net.Addr) PeerInfo {
	vsockAddr, ok := addr.(*vsock.Addr)
	if !ok {
		return PeerInfo{}
	}

	return PeerInfo{CID: vsockAddr.ContextID}
}

func isExpectedShutdownAcceptError(err error) bool {
	if errors.Is(err, net.ErrClosed) {
		return true
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}

	return opErr.Op == "accept" &&
		opErr.Err != nil &&
		strings.Contains(opErr.Err.Error(), "use of closed network connection")
}
