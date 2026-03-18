package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/GJCav/V-reflink/internal/client"
	"github.com/GJCav/V-reflink/internal/config"
	"github.com/GJCav/V-reflink/internal/install"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/validate"
	pkgassets "github.com/GJCav/V-reflink/packaging"
)

type requestFunc func(context.Context, config.CLI, protocol.Request) (*protocol.Response, error)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "vreflink: %s\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	return newRootCmdWithDependencies(config.LoadCLI, os.Getwd, executeRequest)
}

func newRootCmdWithConfig(cfg config.CLI) *cobra.Command {
	return newRootCmdWithDependencies(func() (config.CLI, error) {
		return cfg, nil
	}, os.Getwd, executeRequest)
}

func newRootCmdWithLoader(loadConfig func() (config.CLI, error)) *cobra.Command {
	return newRootCmdWithDependencies(loadConfig, os.Getwd, executeRequest)
}

func newRootCmdWithDependencies(
	loadConfig func() (config.CLI, error),
	getwd func() (string, error),
	doRequest requestFunc,
) *cobra.Command {
	defaults := config.DefaultCLI()

	var recursive bool
	mountRoot := defaults.GuestMountRoot
	hostCID := defaults.HostCID
	port := defaults.VsockPort
	timeout := defaults.Timeout

	cmd := &cobra.Command{
		Use:           "vreflink [-r] SRC DST",
		Short:         "Request a host-side reflink inside a virtiofs share",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveRuntimeConfig(cmd, loadConfig, mountRoot, hostCID, port, timeout)
			if err != nil {
				return err
			}

			cwd, err := getwd()
			if err != nil {
				return fmt.Errorf("resolve working directory: %w", err)
			}

			req, err := buildRequest(cfg, cwd, args[0], args[1], recursive)
			if err != nil {
				return err
			}

			resp, err := doRequest(cmd.Context(), cfg, req)
			if err != nil {
				return err
			}

			if resp.OK {
				return nil
			}

			if resp.Error == nil {
				return errors.New("daemon returned an unknown error")
			}

			return &protocol.CodedError{
				Code:    resp.Error.Code,
				Message: resp.Error.Message,
			}
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "reflink a directory tree")
	cmd.Flags().StringVar(&mountRoot, "mount-root", mountRoot, "guest virtiofs mount root")
	cmd.Flags().Uint32Var(&hostCID, "cid", hostCID, "host vsock CID")
	cmd.Flags().Uint32Var(&port, "port", port, "vsock port")
	cmd.Flags().DurationVar(&timeout, "timeout", timeout, "request timeout")

	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newConfigCmd())

	return cmd
}

func buildRequest(cfg config.CLI, cwd, srcArg, dstArg string, recursive bool) (protocol.Request, error) {
	srcRel, err := validate.ResolveGuestArgument(cfg.GuestMountRoot, cwd, srcArg)
	if err != nil {
		return protocol.Request{}, err
	}

	dstRel, err := validate.ResolveGuestArgument(cfg.GuestMountRoot, cwd, dstArg)
	if err != nil {
		return protocol.Request{}, err
	}

	return protocol.Request{
		Version:   protocol.Version1,
		Op:        protocol.OpReflink,
		Recursive: recursive,
		Src:       srcRel,
		Dst:       dstRel,
	}, nil
}

func executeRequest(ctx context.Context, cfg config.CLI, req protocol.Request) (*protocol.Response, error) {
	cli := client.New(cfg.HostCID, cfg.VsockPort, cfg.Timeout)
	return cli.Do(ctx, req)
}

func resolveRuntimeConfig(cmd *cobra.Command, loadConfig func() (config.CLI, error), mountRoot string, hostCID, port uint32, timeout time.Duration) (config.CLI, error) {
	cfg, err := loadConfig()
	if err != nil {
		return config.CLI{}, fmt.Errorf("load CLI config: %w", err)
	}

	if cmd.Flags().Changed("mount-root") {
		cfg.GuestMountRoot = mountRoot
	}
	if cmd.Flags().Changed("cid") {
		cfg.HostCID = hostCID
	}
	if cmd.Flags().Changed("port") {
		cfg.VsockPort = port
	}
	if cmd.Flags().Changed("timeout") {
		cfg.Timeout = timeout
	}

	return cfg, nil
}

func newInstallCmd() *cobra.Command {
	binDir := "/usr/bin"

	cmd := &cobra.Command{
		Use:           "install",
		Short:         "Install the current guest binary",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			executablePath, err := os.Executable()
			if err != nil {
				return err
			}

			installedPath, err := install.InstallBinary(executablePath, binDir, install.GuestBinaryName)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "installed guest binary to %s\n", installedPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&binDir, "bin-dir", binDir, "directory to install vreflink into")
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "config",
		Short:         "Manage guest configuration",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(newConfigInitCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:           "init",
		Short:         "Write the guest config template to the XDG config path",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.CLIConfigPath()
			if err != nil {
				return err
			}

			if err := install.WriteTemplate(path, pkgassets.GuestConfigTemplate(), 0o644, force); err != nil {
				if errors.Is(err, os.ErrExist) {
					return fmt.Errorf("config file already exists at %s (use --force to overwrite)", path)
				}
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return cmd
}
