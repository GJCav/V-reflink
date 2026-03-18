package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/KarpelesLab/reflink"
	"golang.org/x/sys/unix"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/tree"
	"github.com/GJCav/V-reflink/internal/validate"
)

type Reflinker interface {
	Reflink(srcPath, dstPath string, mode fs.FileMode) error
}

type Service struct {
	Root      string
	Reflinker Reflinker
}

type FileReflinker struct{}

func New(root string) *Service {
	return &Service{
		Root:      root,
		Reflinker: FileReflinker{},
	}
}

func ValidateShareRoot(root string, reflinker Reflinker) error {
	rootPath, err := normalizeShareRoot(root)
	if err != nil {
		return err
	}

	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("share root %q does not exist", rootPath)
		}

		return fmt.Errorf("stat share root %q: %w", rootPath, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("share root %q must be a directory", rootPath)
	}

	if reflinker == nil {
		reflinker = FileReflinker{}
	}

	var srcFile *os.File
	srcFile, err = os.CreateTemp(rootPath, ".vreflinkd-startup-probe-src-*")
	if err != nil {
		return fmt.Errorf("share root %q is not writable: %w", rootPath, err)
	}

	srcPath := srcFile.Name()
	var dstPath string
	defer func() {
		if srcFile != nil {
			_ = srcFile.Close()
		}
		_ = os.Remove(srcPath)
		if dstPath != "" {
			_ = os.Remove(dstPath)
		}
	}()

	if _, err := srcFile.Write([]byte("probe")); err != nil {
		return fmt.Errorf("prepare share root probe in %q: %w", rootPath, err)
	}

	if err := srcFile.Close(); err != nil {
		return fmt.Errorf("prepare share root probe in %q: %w", rootPath, err)
	}
	srcFile = nil

	dstFile, err := os.CreateTemp(rootPath, ".vreflinkd-startup-probe-dst-*")
	if err != nil {
		return fmt.Errorf("share root %q is not writable: %w", rootPath, err)
	}

	dstPath = dstFile.Name()
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("prepare share root probe in %q: %w", rootPath, err)
	}

	if err := os.Remove(dstPath); err != nil {
		return fmt.Errorf("prepare share root probe in %q: %w", rootPath, err)
	}

	if err := reflinker.Reflink(srcPath, dstPath, 0o600); err != nil {
		if coded, ok := protocol.AsCoded(err); ok && coded.Code == protocol.CodeEOPNOTSUPP {
			return fmt.Errorf("share root %q does not support reflink: %w", rootPath, err)
		}

		switch protocol.CodeFromError(err) {
		case protocol.CodeEOPNOTSUPP:
			return fmt.Errorf("share root %q does not support reflink: %w", rootPath, err)
		default:
			return fmt.Errorf("share root %q failed reflink probe: %w", rootPath, err)
		}
	}

	return nil
}

func (s *Service) Execute(req protocol.Request) error {
	if err := validate.Request(req); err != nil {
		return err
	}

	switch {
	case req.Recursive:
		return s.reflinkTree(req.Src, req.Dst)
	default:
		return s.reflinkSingle(req.Src, req.Dst)
	}
}

func (s *Service) reflinkSingle(srcRel, dstRel string) error {
	srcPath, srcInfo, err := validate.ResolveSource(s.Root, srcRel)
	if err != nil {
		return err
	}

	if err := validate.RequireRegularFile(srcInfo); err != nil {
		return err
	}

	if err := validate.RejectHardlink(srcInfo); err != nil {
		return err
	}

	dstPath, _, err := validate.ResolveDestination(s.Root, dstRel)
	if err != nil {
		return err
	}

	return s.reflinker().Reflink(srcPath, dstPath, srcInfo.Mode().Perm())
}

func (s *Service) reflinkTree(srcRel, dstRel string) error {
	srcPath, srcInfo, err := validate.ResolveSource(s.Root, srcRel)
	if err != nil {
		return err
	}

	if err := validate.RequireDirectory(srcInfo); err != nil {
		return err
	}

	dstPath, _, err := validate.ResolveDestination(s.Root, dstRel)
	if err != nil {
		return err
	}

	walker := tree.Walker{Reflinker: s.reflinker()}
	return walker.ReflinkTree(srcPath, dstPath)
}

func (s *Service) reflinker() Reflinker {
	if s.Reflinker != nil {
		return s.Reflinker
	}

	return FileReflinker{}
}

func (FileReflinker) Reflink(srcPath, dstPath string, mode fs.FileMode) (err error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}

	cleanup := true
	defer func() {
		if closeErr := dst.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if cleanup {
			_ = os.Remove(dstPath)
		}
	}()

	if err := reflink.Reflink(dst, src, false); err != nil {
		return mapReflinkError(err)
	}

	if err := dst.Chmod(mode.Perm()); err != nil {
		return err
	}

	cleanup = false
	return nil
}

func mapReflinkError(err error) error {
	switch {
	case errors.Is(err, reflink.ErrReflinkUnsupported), errors.Is(err, reflink.ErrReflinkFailed),
		errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.ENOTSUP), errors.Is(err, unix.EXDEV):
		return protocol.WrapError(protocol.CodeEOPNOTSUPP, "reflink is not supported for this source and destination", err)
	default:
		return err
	}
}

func normalizeShareRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("share root is required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve share root: %w", err)
	}

	return filepath.Clean(absRoot), nil
}
