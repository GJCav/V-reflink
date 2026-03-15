package validate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/GJCav/V-reflink/internal/protocol"
)

func Request(req protocol.Request) error {
	if err := req.Validate(); err != nil {
		return err
	}

	if _, err := NormalizeRelative(req.Src); err != nil {
		return err
	}

	if _, err := NormalizeRelative(req.Dst); err != nil {
		return err
	}

	return nil
}

func GuestToRelative(mountRoot, guestPath string) (string, error) {
	root, err := normalizeRoot(mountRoot)
	if err != nil {
		return "", err
	}

	cleanGuest := filepath.Clean(guestPath)
	if !filepath.IsAbs(cleanGuest) {
		return "", protocol.NewError(protocol.CodeEPERM, "guest paths must be absolute")
	}

	rel, err := filepath.Rel(root, cleanGuest)
	if err != nil {
		return "", protocol.WrapError(protocol.CodeEPERM, "path must stay within the guest mount root", err)
	}

	normalized, err := NormalizeRelative(rel)
	if err != nil {
		return "", protocol.NewError(protocol.CodeEPERM, "path must stay within the guest mount root")
	}

	return normalized, nil
}

func NormalizeRelative(path string) (string, error) {
	if path == "" {
		return "", protocol.NewError(protocol.CodeEINVAL, "path is required")
	}

	clean := filepath.Clean(filepath.FromSlash(path))
	switch {
	case clean == ".":
		return ".", nil
	case filepath.IsAbs(clean):
		return "", protocol.NewError(protocol.CodeEPERM, "absolute paths are not allowed")
	case clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)):
		return "", protocol.NewError(protocol.CodeEPERM, "path must stay within the shared root")
	default:
		return filepath.ToSlash(clean), nil
	}
}

func ResolveSource(root, rel string) (string, fs.FileInfo, error) {
	rootPath, cleanRel, resolved, err := resolvePath(root, rel)
	if err != nil {
		return "", nil, err
	}

	if err := rejectSymlinkComponents(rootPath, cleanRel, false, "source not found"); err != nil {
		return "", nil, err
	}

	info, err := os.Lstat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, protocol.WrapError(protocol.CodeENOENT, "source not found", err)
		}
		return "", nil, err
	}

	return resolved, info, nil
}

func ResolveDestination(root, rel string) (string, string, error) {
	rootPath, cleanRel, resolved, err := resolvePath(root, rel)
	if err != nil {
		return "", "", err
	}

	if err := rejectSymlinkComponents(rootPath, cleanRel, true, "destination parent does not exist"); err != nil {
		return "", "", err
	}

	if _, err := os.Lstat(resolved); err == nil {
		return "", "", protocol.NewError(protocol.CodeEEXIST, "destination already exists")
	} else if !os.IsNotExist(err) {
		return "", "", err
	}

	parentRel := filepath.Dir(filepath.FromSlash(cleanRel))
	parentPath, err := securejoin.SecureJoin(rootPath, parentRel)
	if err != nil {
		return "", "", protocol.WrapError(protocol.CodeEPERM, "path must stay within the shared root", err)
	}

	parentInfo, err := os.Lstat(parentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", protocol.WrapError(protocol.CodeENOENT, "destination parent does not exist", err)
		}
		return "", "", err
	}

	if parentInfo.Mode()&os.ModeSymlink != 0 {
		return "", "", protocol.NewError(protocol.CodeEINVAL, "symlinks are not supported")
	}

	if !parentInfo.IsDir() {
		return "", "", protocol.NewError(protocol.CodeEINVAL, "destination parent must be a directory")
	}

	return resolved, parentPath, nil
}

func RequireRegularFile(info fs.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return protocol.NewError(protocol.CodeEINVAL, "symlinks are not supported")
	case info.Mode().IsRegular():
		return nil
	default:
		return protocol.NewError(protocol.CodeEINVAL, "source must be a regular file")
	}
}

func RequireDirectory(info fs.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return protocol.NewError(protocol.CodeEINVAL, "symlinks are not supported")
	case info.IsDir():
		return nil
	default:
		return protocol.NewError(protocol.CodeEINVAL, "source must be a directory")
	}
}

func RejectHardlink(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}

	if stat.Nlink > 1 {
		return protocol.NewError(protocol.CodeEINVAL, "hard links are not supported")
	}

	return nil
}

func resolvePath(root, rel string) (string, string, string, error) {
	rootPath, err := normalizeRoot(root)
	if err != nil {
		return "", "", "", err
	}

	cleanRel, err := NormalizeRelative(rel)
	if err != nil {
		return "", "", "", err
	}

	resolved, err := securejoin.SecureJoin(rootPath, cleanRel)
	if err != nil {
		return "", "", "", protocol.WrapError(protocol.CodeEPERM, "path must stay within the shared root", err)
	}

	return rootPath, cleanRel, resolved, nil
}

func normalizeRoot(root string) (string, error) {
	if root == "" {
		return "", protocol.NewError(protocol.CodeEINVAL, "share root is required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve share root: %w", err)
	}

	return filepath.Clean(absRoot), nil
}

func rejectSymlinkComponents(root, rel string, allowMissingLeaf bool, missingMessage string) error {
	if rel == "." {
		return nil
	}

	current := root
	parts := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)

		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				if allowMissingLeaf && i == len(parts)-1 {
					return nil
				}
				return protocol.WrapError(protocol.CodeENOENT, missingMessage, err)
			}

			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return protocol.NewError(protocol.CodeEINVAL, "symlinks are not supported")
		}
	}

	return nil
}
