package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/mdlayher/vsock"
	"github.com/spf13/cobra"

	"github.com/GJCav/V-reflink/internal/auth"
	"github.com/GJCav/V-reflink/internal/config"
	"github.com/GJCav/V-reflink/internal/install"
	"github.com/GJCav/V-reflink/internal/logutil"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/server"
	"github.com/GJCav/V-reflink/internal/service"
	pkgassets "github.com/GJCav/V-reflink/packaging"
)

var (
	validateShareRoot     = service.ValidateShareRoot
	loadDaemonTokenMap    = auth.LoadTokenMap
	resolveExecutablePath = os.Executable
	listenVsock           = func(port uint32) (net.Listener, error) {
		return vsock.Listen(port, nil)
	}
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
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return runDaemon(ctx, cfg, logLevel)
		},
	}

	cmd.Flags().StringVar(&cfg.ShareRoot, "share-root", cfg.ShareRoot, "host share root")
	cmd.Flags().Uint32Var(&cfg.VsockPort, "port", cfg.VsockPort, "vsock port")
	cmd.Flags().DurationVar(&cfg.ReadTimeout, "read-timeout", cfg.ReadTimeout, "connection read timeout")
	cmd.Flags().DurationVar(&cfg.WriteTimeout, "write-timeout", cfg.WriteTimeout, "connection write timeout")
	cmd.Flags().StringVar(&cfg.TokenMapPath, "token-map-path", cfg.TokenMapPath, "path to the YAML token map")
	cmd.Flags().StringVar(&logLevel, "log-level", logLevel, "log level (debug, info, warn, error)")

	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newSystemdUnitCmd())
	cmd.AddCommand(newWorkerCmd())

	return cmd
}

func runDaemon(ctx context.Context, cfg config.Daemon, logLevel string) error {
	logger := logutil.NewLogger(logutil.ParseLevel(logLevel))

	if err := validateShareRoot(cfg.ShareRoot, service.FileReflinker{}); err != nil {
		return err
	}

	tokenMap, err := loadRuntimeTokenMap(cfg)
	if err != nil {
		return err
	}
	if tokenMap == nil {
		logger.Warn("starting without token authentication because VREFLINK_ALLOW_V1_FALLBACK=true")
	}

	executor, err := newDaemonExecutor(cfg, tokenMap)
	if err != nil {
		return err
	}

	listener, err := listenVsock(cfg.VsockPort)
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
			err := executor.Execute(ctx, req)
			resp := protocol.ResponseFromError(err)

			result := "OK"
			resultMessage := ""
			if !resp.OK && resp.Error != nil {
				result = resp.Error.Code
				resultMessage = resp.Error.Message
			}

			logger.Info(
				"request",
				slog.Uint64("cid", uint64(peer.CID)),
				slog.String("op", req.Op),
				slog.String("src", req.Src),
				slog.String("dst", req.Dst),
				slog.String("result", result),
				slog.String("message", resultMessage),
			)

			return resp
		},
	}

	return srv.Serve(ctx)
}

func loadRuntimeTokenMap(cfg config.Daemon) (*auth.TokenMap, error) {
	tokenMap, err := loadDaemonTokenMap(cfg.TokenMapPath)
	if err != nil {
		return nil, err
	}
	if tokenMap != nil {
		return tokenMap, nil
	}

	if cfg.AllowV1Fallback {
		return nil, nil
	}

	if cfg.TokenMapPath == "" {
		return nil, fmt.Errorf("token map path is required unless VREFLINK_ALLOW_V1_FALLBACK=true")
	}

	return nil, fmt.Errorf("token map %q is required unless VREFLINK_ALLOW_V1_FALLBACK=true", cfg.TokenMapPath)
}

func newDaemonExecutor(cfg config.Daemon, tokenMap *auth.TokenMap) (daemonExecutor, error) {
	executor := daemonExecutor{
		service:  service.New(cfg.ShareRoot),
		tokenMap: tokenMap,
	}

	if tokenMap == nil {
		return executor, nil
	}

	executablePath, err := resolveExecutablePath()
	if err != nil {
		return daemonExecutor{}, fmt.Errorf("resolve daemon executable: %w", err)
	}

	worker, err := newCommandWorker(executablePath, cfg.ShareRoot)
	if err != nil {
		return daemonExecutor{}, fmt.Errorf("resolve current credentials: %w", err)
	}

	executor.worker = worker
	return executor, nil
}

func newWorkerCmd() *cobra.Command {
	var shareRoot string

	cmd := &cobra.Command{
		Use:           "worker",
		Hidden:        true,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runWorker(shareRoot, os.Stdin, os.Stdout); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&shareRoot, "share-root", "", "host share root")
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
