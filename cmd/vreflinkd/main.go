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
	"github.com/GJCav/V-reflink/internal/install"
	"github.com/GJCav/V-reflink/internal/logutil"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/server"
	"github.com/GJCav/V-reflink/internal/service"
	pkgassets "github.com/GJCav/V-reflink/packaging"
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

	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newSystemdUnitCmd())

	return cmd
}

func newInstallCmd() *cobra.Command {
	binDir := "/usr/bin"
	systemdDir := "/etc/systemd/system"
	defaultsPath := "/etc/default/vreflinkd"

	cmd := &cobra.Command{
		Use:           "install",
		Short:         "Install the current host binary and templates",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			executablePath, err := os.Executable()
			if err != nil {
				return err
			}

			result, err := install.InstallHost(
				executablePath,
				binDir,
				systemdDir,
				defaultsPath,
				pkgassets.SystemdUnitTemplate(),
				pkgassets.DaemonDefaultsTemplate(),
			)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "installed host binary to %s\n", result.BinaryPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "installed systemd unit to %s\n", result.SystemdPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "installed defaults file to %s\n", result.DefaultsPath)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "next steps:")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  sudo systemctl daemon-reload")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  sudo systemctl enable --now vreflinkd")
			return nil
		},
	}

	cmd.Flags().StringVar(&binDir, "bin-dir", binDir, "directory to install vreflinkd into")
	cmd.Flags().StringVar(&systemdDir, "systemd-dir", systemdDir, "directory to install the systemd unit into")
	cmd.Flags().StringVar(&defaultsPath, "defaults-path", defaultsPath, "path to install the defaults file into")
	return cmd
}

func newSystemdUnitCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "systemd-unit",
		Short:         "Print the canonical systemd unit template",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(pkgassets.SystemdUnitTemplate())
			return err
		},
	}
}
