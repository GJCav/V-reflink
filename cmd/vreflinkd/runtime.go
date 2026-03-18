package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"syscall"

	"github.com/GJCav/V-reflink/internal/auth"
	"github.com/GJCav/V-reflink/internal/framing"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/service"
)

type requestService interface {
	Execute(protocol.Request) error
}

type workerInvoker interface {
	Execute(context.Context, protocol.Request, auth.Identity) error
}

type daemonExecutor struct {
	service  requestService
	tokenMap *auth.TokenMap
	worker   workerInvoker
}

var newWorkerService = func(shareRoot string) requestService {
	return service.New(shareRoot)
}

func (e daemonExecutor) Execute(ctx context.Context, req protocol.Request) error {
	if e.tokenMap == nil {
		if req.Version != protocol.Version1 {
			if err := req.Validate(); err != nil {
				return err
			}
			return protocol.NewError(protocol.CodeEINVAL, "token authentication is not configured")
		}

		return e.service.Execute(req)
	}

	if err := req.Validate(); err != nil {
		return err
	}

	if req.Version != protocol.Version2 {
		return protocol.NewError(protocol.CodeEINVAL, "token-authenticated requests require protocol version 2")
	}

	identity, ok := e.tokenMap.Resolve(req.Token)
	if !ok {
		return protocol.NewError(protocol.CodeEPERM, "invalid authentication token")
	}

	return e.worker.Execute(ctx, req, identity)
}

type commandWorker struct {
	executablePath  string
	shareRoot       string
	currentIdentity auth.Identity
}

func newCommandWorker(executablePath, shareRoot string) (*commandWorker, error) {
	currentIdentity, err := currentProcessIdentity()
	if err != nil {
		return nil, err
	}

	return &commandWorker{
		executablePath:  executablePath,
		shareRoot:       shareRoot,
		currentIdentity: currentIdentity,
	}, nil
}

func (w *commandWorker) Execute(ctx context.Context, req protocol.Request, identity auth.Identity) error {
	cmd := exec.CommandContext(ctx, w.executablePath, "worker", "--share-root", w.shareRoot)
	if credential := credentialForIdentity(identity, w.currentIdentity); credential != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: credential,
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open worker stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open worker stdout: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}

	writeErr := framing.Write(stdin, req)
	closeErr := stdin.Close()
	if writeErr == nil && closeErr != nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		waitErr := cmd.Wait()
		if waitErr != nil {
			return fmt.Errorf("write worker request: %w (%s)", waitErr, stderr.String())
		}
		return fmt.Errorf("write worker request: %w", writeErr)
	}

	var resp protocol.Response
	readErr := framing.Read(stdout, &resp)
	waitErr := cmd.Wait()
	if readErr != nil {
		if waitErr != nil {
			return fmt.Errorf("read worker response: %w (%s)", waitErr, stderr.String())
		}
		return fmt.Errorf("read worker response: %w", readErr)
	}
	if waitErr != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("worker failed: %w (%s)", waitErr, stderr.String())
		}
		return fmt.Errorf("worker failed: %w", waitErr)
	}

	if resp.OK {
		return nil
	}
	if resp.Error == nil {
		return fmt.Errorf("worker returned an unknown error")
	}

	return &protocol.CodedError{
		Code:    resp.Error.Code,
		Message: resp.Error.Message,
	}
}

func credentialForIdentity(identity, current auth.Identity) *syscall.Credential {
	if identity.UID == current.UID &&
		identity.GID == current.GID &&
		sameGroupSet(identity.Groups, current.Groups) {
		return nil
	}

	return &syscall.Credential{
		Uid:    identity.UID,
		Gid:    identity.GID,
		Groups: append([]uint32(nil), identity.Groups...),
	}
}

func currentProcessIdentity() (auth.Identity, error) {
	primaryGroup := uint32(os.Getgid())
	groups, err := os.Getgroups()
	if err != nil {
		return auth.Identity{}, fmt.Errorf("resolve current groups: %w", err)
	}

	normalizedGroups := make([]uint32, 0, len(groups))
	for _, group := range groups {
		if group < 0 {
			continue
		}

		groupID := uint32(group)
		if groupID == primaryGroup {
			continue
		}
		normalizedGroups = append(normalizedGroups, groupID)
	}

	return auth.Identity{
		UID:    uint32(os.Getuid()),
		GID:    primaryGroup,
		Groups: normalizedGroups,
	}, nil
}

func sameGroupSet(left, right []uint32) bool {
	if len(left) != len(right) {
		return false
	}

	leftCopy := append([]uint32(nil), left...)
	rightCopy := append([]uint32(nil), right...)
	slices.Sort(leftCopy)
	slices.Sort(rightCopy)
	return slices.Equal(leftCopy, rightCopy)
}

func runWorker(shareRoot string, input io.Reader, output io.Writer) error {
	var req protocol.Request
	if err := framing.Read(input, &req); err != nil {
		return framing.Write(output, protocol.ResponseFromError(protocol.WrapError(protocol.CodeEINVAL, "invalid request", err)))
	}

	return framing.Write(output, protocol.ResponseFromError(newWorkerService(shareRoot).Execute(req)))
}
