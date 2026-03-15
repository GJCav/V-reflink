package service

import (
	"errors"
	"io/fs"
	"os"

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
