package tree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/validate"
)

const (
	temporaryDirectoryMode = 0o700
	preservedDirectoryMode = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
)

type Reflinker interface {
	Reflink(srcPath, dstPath string, mode fs.FileMode) error
}

type Walker struct {
	Reflinker Reflinker
}

type pendingDirectoryMode struct {
	path string
	mode fs.FileMode
}

func (w Walker) ReflinkTree(srcRoot, dstRoot string) error {
	srcInfo, err := os.Lstat(srcRoot)
	if err != nil {
		return err
	}

	if !srcInfo.IsDir() {
		return protocol.NewError(protocol.CodeEINVAL, "source must be a directory")
	}

	if err := os.Mkdir(dstRoot, temporaryDirectoryMode); err != nil {
		return err
	}

	directories := []pendingDirectoryMode{{
		path: dstRoot,
		mode: exactDirectoryMode(srcInfo.Mode()),
	}}

	walkErr := filepath.WalkDir(srcRoot, func(path string, entry fs.DirEntry, walkErr error) error {
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
			if err := os.Mkdir(targetPath, temporaryDirectoryMode); err != nil {
				return err
			}
			directories = append(directories, pendingDirectoryMode{
				path: targetPath,
				mode: exactDirectoryMode(info.Mode()),
			})
			return nil
		case info.Mode().IsRegular():
			if err := validate.RejectHardlink(info); err != nil {
				return err
			}
			return w.Reflinker.Reflink(path, targetPath, info.Mode().Perm())
		default:
			return protocol.NewError(protocol.CodeEINVAL, "unsupported file type")
		}
	})

	chmodErr := restoreDirectoryModes(directories)
	if walkErr != nil {
		if chmodErr != nil {
			return errors.Join(walkErr, chmodErr)
		}
		return walkErr
	}

	return chmodErr
}

func exactDirectoryMode(mode fs.FileMode) fs.FileMode {
	return mode & preservedDirectoryMode
}

func restoreDirectoryModes(directories []pendingDirectoryMode) error {
	for index := len(directories) - 1; index >= 0; index-- {
		directory := directories[index]
		if err := os.Chmod(directory.path, directory.mode); err != nil {
			return err
		}
	}

	return nil
}
