package tree

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/validate"
)

type Reflinker interface {
	Reflink(srcPath, dstPath string, mode fs.FileMode) error
}

type Walker struct {
	Reflinker Reflinker
}

func (w Walker) ReflinkTree(srcRoot, dstRoot string) error {
	srcInfo, err := os.Lstat(srcRoot)
	if err != nil {
		return err
	}

	if !srcInfo.IsDir() {
		return protocol.NewError(protocol.CodeEINVAL, "source must be a directory")
	}

	if err := os.Mkdir(dstRoot, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	return filepath.WalkDir(srcRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path == srcRoot {
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dstRoot, rel)

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return protocol.NewError(protocol.CodeEINVAL, "symlinks are not supported")
		case info.IsDir():
			return os.Mkdir(targetPath, info.Mode().Perm())
		case info.Mode().IsRegular():
			if err := validate.RejectHardlink(info); err != nil {
				return err
			}
			return w.Reflinker.Reflink(path, targetPath, info.Mode().Perm())
		default:
			return protocol.NewError(protocol.CodeEINVAL, "unsupported file type")
		}
	})
}
