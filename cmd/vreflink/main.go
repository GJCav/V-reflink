package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/GJCav/V-reflink/internal/client"
	"github.com/GJCav/V-reflink/internal/config"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/validate"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "vreflink: %s\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cfg := config.LoadCLI()
	var recursive bool

	cmd := &cobra.Command{
		Use:           "vreflink [-r] SRC DST",
		Short:         "Request a host-side reflink inside a virtiofs share",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcRel, err := validate.GuestToRelative(cfg.GuestMountRoot, args[0])
			if err != nil {
				return err
			}

			dstRel, err := validate.GuestToRelative(cfg.GuestMountRoot, args[1])
			if err != nil {
				return err
			}

			req := protocol.Request{
				Version:   protocol.Version1,
				Op:        protocol.OpReflink,
				Recursive: recursive,
				Src:       srcRel,
				Dst:       dstRel,
			}

			cli := client.New(cfg.HostCID, cfg.VsockPort, cfg.Timeout)
			resp, err := cli.Do(cmd.Context(), req)
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
	cmd.Flags().StringVar(&cfg.GuestMountRoot, "mount-root", cfg.GuestMountRoot, "guest virtiofs mount root")
	cmd.Flags().Uint32Var(&cfg.HostCID, "cid", cfg.HostCID, "host vsock CID")
	cmd.Flags().Uint32Var(&cfg.VsockPort, "port", cfg.VsockPort, "vsock port")
	cmd.Flags().DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "request timeout")

	return cmd
}
