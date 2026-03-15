package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mdlayher/vsock"
	"github.com/spf13/cobra"

	"github.com/GJCav/V-reflink/internal/config"
	"github.com/GJCav/V-reflink/internal/logutil"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/server"
	"github.com/GJCav/V-reflink/internal/service"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "vreflinkd: %s\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cfg := config.LoadDaemon()
	logLevel := "info"

	cmd := &cobra.Command{
		Use:           "vreflinkd",
		Short:         "Serve host-side reflink requests over vsock",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := logutil.NewLogger(logutil.ParseLevel(logLevel))
			svc := service.New(cfg.ShareRoot)

			listener, err := vsock.Listen(cfg.VsockPort, nil)
			if err != nil {
				return err
			}

			logger.Info("listening", "share_root", cfg.ShareRoot, "port", cfg.VsockPort)

			srv := &server.Server{
				Listener:     listener,
				Logger:       logger,
				ReadTimeout:  cfg.ReadTimeout,
				WriteTimeout: cfg.WriteTimeout,
				Handler: func(ctx context.Context, req protocol.Request, peer server.PeerInfo) protocol.Response {
					err := svc.Execute(req)
					resp := protocol.ResponseFromError(err)

					result := "OK"
					if !resp.OK && resp.Error != nil {
						result = resp.Error.Code
					}

					logger.Info(
						"request",
						slog.Uint64("cid", uint64(peer.CID)),
						slog.String("op", req.Op),
						slog.String("src", req.Src),
						slog.String("dst", req.Dst),
						slog.String("result", result),
					)

					return resp
				},
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return srv.Serve(ctx)
		},
	}

	cmd.Flags().StringVar(&cfg.ShareRoot, "share-root", cfg.ShareRoot, "host share root")
	cmd.Flags().Uint32Var(&cfg.VsockPort, "port", cfg.VsockPort, "vsock port")
	cmd.Flags().DurationVar(&cfg.ReadTimeout, "read-timeout", cfg.ReadTimeout, "connection read timeout")
	cmd.Flags().DurationVar(&cfg.WriteTimeout, "write-timeout", cfg.WriteTimeout, "connection write timeout")
	cmd.Flags().StringVar(&logLevel, "log-level", logLevel, "log level (debug, info, warn, error)")

	return cmd
}
