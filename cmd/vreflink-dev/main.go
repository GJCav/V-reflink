package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/GJCav/V-reflink/internal/devsupport"
	"github.com/GJCav/V-reflink/internal/releasebuild"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

type suitePreparation struct {
	env     []string
	cleanup func(context.Context) error
}

type deps struct {
	repoRoot            func() (string, error)
	runCommand          func(context.Context, string, []string, io.Writer, io.Writer, string, ...string) error
	buildRelease        func(context.Context, releasebuild.Options) (releasebuild.Artifacts, error)
	checkVMPrereqs      func(context.Context) []string
	prepareReflinkSuite func(context.Context, string) (suitePreparation, error)
	prepareVMSuite      func(context.Context, string) (suitePreparation, error)
}

func defaultDeps() deps {
	return deps{
		repoRoot:            devsupport.SourceRepoRoot,
		runCommand:          devsupport.RunCommandStreaming,
		buildRelease:        releasebuild.Build,
		checkVMPrereqs:      testsupport.CheckVMPrereqs,
		prepareReflinkSuite: defaultPrepareReflinkSuite,
		prepareVMSuite:      defaultPrepareVMSuite,
	}
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "vreflink-dev: %s\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	return newRootCmdWithDeps(defaultDeps())
}

func newRootCmdWithDeps(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vreflink-dev",
		Short:         "Development runner for tests, VM integration, and release packaging",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(newTestCmd(d))
	cmd.AddCommand(newReleaseCmd(d))
	cmd.AddCommand(newVMCmd(d))
	return cmd
}

func newTestCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "test",
		Short:         "Run project test suites",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.PersistentFlags().Bool("race", false, "enable the Go race detector where supported")
	cmd.AddCommand(
		newGoTestSuiteCmd(d, "quick", []string{"test", "./..."}, true, nil),
		newGoTestSuiteCmd(d, "reflinkfs", []string{"test", "-tags", "reflinkfstest", "./internal/service"}, true, d.prepareReflinkSuite),
		newGoTestSuiteCmd(d, "vm", []string{"test", "-count=1", "-tags", "vmtest", "./integration/vm"}, false, d.prepareVMSuite),
		newGoTestSuiteCmd(d, "release", []string{"test", "-count=1", "-tags", "releasetest", "./integration/release"}, false, nil),
		newAllTestsCmd(d),
	)
	return cmd
}

func newGoTestSuiteCmd(d deps, suite string, baseArgs []string, allowRace bool, prepare func(context.Context, string) (suitePreparation, error)) *cobra.Command {
	return &cobra.Command{
		Use:           suite,
		Short:         fmt.Sprintf("Run the %s test suite", suite),
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			race, err := cmd.Flags().GetBool("race")
			if err != nil {
				return err
			}
			if race && !allowRace {
				switch suite {
				case "vm":
					return errors.New("the vm suite does not support --race; use 'test quick --race' or 'test reflinkfs --race' instead")
				case "release":
					return errors.New("the release suite does not support --race")
				default:
					return fmt.Errorf("the %s suite does not support --race", suite)
				}
			}

			args := append([]string(nil), baseArgs...)
			if race {
				args = append([]string{"test", "-race"}, args[1:]...)
			}
			return runPreparedGoSuite(cmd, d, args, prepare)
		},
	}
}

func newAllTestsCmd(d deps) *cobra.Command {
	return &cobra.Command{
		Use:           "all",
		Short:         "Run quick, reflinkfs, then vm",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			race, err := cmd.Flags().GetBool("race")
			if err != nil {
				return err
			}
			if race {
				return errors.New("the all suite does not support --race because the vm and release suites do not run with race; run quick --race and reflinkfs --race separately")
			}

			for _, spec := range []struct {
				args    []string
				prepare func(context.Context, string) (suitePreparation, error)
			}{
				{args: []string{"test", "./..."}},
				{args: []string{"test", "-tags", "reflinkfstest", "./internal/service"}, prepare: d.prepareReflinkSuite},
				{args: []string{"test", "-count=1", "-tags", "vmtest", "./integration/vm"}, prepare: d.prepareVMSuite},
			} {
				if err := runPreparedGoSuite(cmd, d, spec.args, spec.prepare); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newReleaseCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "release",
		Short:         "Build release artifacts",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(newReleaseBuildCmd(d))
	return cmd
}

func newReleaseBuildCmd(d deps) *cobra.Command {
	opts := releasebuild.Options{
		Arch: releasebuild.SupportedArch,
	}

	cmd := &cobra.Command{
		Use:           "build",
		Short:         "Build local release artifacts",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			artifacts, err := d.buildRelease(cmd.Context(), opts)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", artifacts.TarballPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", artifacts.DebPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", artifacts.ChecksumsPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Version, "version", "", "release version")
	_ = cmd.MarkFlagRequired("version")
	cmd.Flags().StringVar(&opts.Arch, "arch", opts.Arch, "target architecture")
	cmd.Flags().StringVar(&opts.OutDir, "out-dir", opts.OutDir, "artifact output directory")
	return cmd
}

func newVMCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vm",
		Short:         "VM-test helpers",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(&cobra.Command{
		Use:           "check-prereqs",
		Short:         "Validate VM integration prerequisites",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			issues := d.checkVMPrereqs(cmd.Context())
			if len(issues) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "vm prerequisites look good")
				return nil
			}
			for _, issue := range issues {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), issue)
			}
			return errors.New("vm prerequisites are not satisfied")
		},
	})

	return cmd
}

func runGo(cmd *cobra.Command, d deps, args ...string) error {
	return runGoWithEnv(cmd, d, nil, args...)
}

func runGoWithEnv(cmd *cobra.Command, d deps, extraEnv []string, args ...string) error {
	repoRoot, err := d.repoRoot()
	if err != nil {
		return err
	}

	return d.runCommand(cmd.Context(), repoRoot, mergeEnv(extraEnv), cmd.OutOrStdout(), cmd.ErrOrStderr(), "go", args...)
}

func runPreparedGoSuite(cmd *cobra.Command, d deps, args []string, prepare func(context.Context, string) (suitePreparation, error)) (err error) {
	if prepare == nil {
		return runGo(cmd, d, args...)
	}

	repoRoot, err := d.repoRoot()
	if err != nil {
		return err
	}

	prepared, err := prepare(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}
	if prepared.cleanup != nil {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cleanupErr := prepared.cleanup(cleanupCtx)
			if cleanupErr == nil {
				return
			}
			if err == nil {
				err = cleanupErr
				return
			}
			err = fmt.Errorf("%w\ncleanup error: %v", err, cleanupErr)
		}()
	}

	return d.runCommand(cmd.Context(), repoRoot, mergeEnv(prepared.env), cmd.OutOrStdout(), cmd.ErrOrStderr(), "go", args...)
}

func defaultPrepareReflinkSuite(ctx context.Context, repoRoot string) (suitePreparation, error) {
	if root := os.Getenv(testsupport.ReflinkTestRootEnv); root != "" {
		if err := testsupport.RequirePrivilegedSuiteAccess(ctx, "reflinkfs", "go run ./cmd/vreflink-dev test reflinkfs"); err != nil {
			return suitePreparation{}, err
		}
		if err := testsupport.ValidateReflinkRoot(root); err != nil {
			return suitePreparation{}, fmt.Errorf("%s=%q is invalid: %w", testsupport.ReflinkTestRootEnv, root, err)
		}
		return suitePreparation{}, nil
	}

	if issues := testsupport.CheckReflinkFSPrereqs(ctx); len(issues) != 0 {
		return suitePreparation{}, formatPrereqError("reflinkfs", issues)
	}

	root, cleanup, err := testsupport.PrepareReflinkTestRoot(ctx, repoRoot)
	if err != nil {
		return suitePreparation{}, err
	}

	return suitePreparation{
		env:     []string{testsupport.ReflinkTestRootEnv + "=" + root},
		cleanup: cleanup,
	}, nil
}

func defaultPrepareVMSuite(ctx context.Context, repoRoot string) (suitePreparation, error) {
	if issues := testsupport.CheckVMPrereqs(ctx); len(issues) != 0 {
		return suitePreparation{}, formatPrereqError("vm", issues)
	}

	if shareRoot := os.Getenv("VREFLINK_VM_SHARE_ROOT"); shareRoot != "" {
		if err := testsupport.ValidateReflinkRoot(shareRoot); err != nil {
			return suitePreparation{}, fmt.Errorf("VREFLINK_VM_SHARE_ROOT=%q is invalid: %w", shareRoot, err)
		}
		return suitePreparation{}, nil
	}

	root, cleanup, err := testsupport.PrepareVMShareRoot(ctx, repoRoot)
	if err != nil {
		return suitePreparation{}, err
	}

	return suitePreparation{
		env:     []string{"VREFLINK_VM_SHARE_ROOT=" + root},
		cleanup: cleanup,
	}, nil
}

func formatPrereqError(suite string, issues []string) error {
	return fmt.Errorf("%s prerequisites are not satisfied:\n%s", suite, strings.Join(issues, "\n"))
}

func mergeEnv(extra []string) []string {
	if len(extra) == 0 {
		return nil
	}

	env := append([]string(nil), os.Environ()...)
	for _, kv := range extra {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			env = append(env, kv)
			continue
		}

		replaced := false
		for i, entry := range env {
			if strings.HasPrefix(entry, key+"=") {
				env[i] = kv
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, kv)
		}
	}

	return env
}
