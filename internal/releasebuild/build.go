package releasebuild

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GJCav/V-reflink/internal/devsupport"
	pkgassets "github.com/GJCav/V-reflink/packaging"
)

const SupportedArch = "amd64"

type Options struct {
	Version string
	Arch    string
	OutDir  string
}

type Artifacts struct {
	TarballPath   string
	DebPath       string
	ChecksumsPath string
}

func Build(ctx context.Context, opts Options) (Artifacts, error) {
	if strings.TrimSpace(opts.Version) == "" {
		return Artifacts{}, fmt.Errorf("--version is required")
	}

	arch := opts.Arch
	if arch == "" {
		arch = SupportedArch
	}
	if arch != SupportedArch {
		return Artifacts{}, fmt.Errorf("only %s is supported in the current release pipeline", SupportedArch)
	}

	repoRoot, err := devsupport.SourceRepoRoot()
	if err != nil {
		return Artifacts{}, err
	}

	outDir := opts.OutDir
	if outDir == "" {
		outDir = filepath.Join(repoRoot, "dist")
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return Artifacts{}, fmt.Errorf("resolve output directory: %w", err)
	}

	if _, err := execLookPath("go"); err != nil {
		return Artifacts{}, fmt.Errorf("missing go: %w", err)
	}
	if _, err := execLookPath("dpkg-deb"); err != nil {
		return Artifacts{}, fmt.Errorf("missing dpkg-deb: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(repoRoot, ".tmp"), 0o755); err != nil {
		return Artifacts{}, fmt.Errorf("create temp root: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Artifacts{}, fmt.Errorf("create output directory: %w", err)
	}

	workRoot, err := os.MkdirTemp(filepath.Join(repoRoot, ".tmp"), "release-build.")
	if err != nil {
		return Artifacts{}, fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(workRoot)

	buildRoot := filepath.Join(workRoot, "build")
	tarRootName := fmt.Sprintf("vreflink_%s_linux_%s", opts.Version, arch)
	tarRoot := filepath.Join(workRoot, tarRootName)
	debRoot := filepath.Join(workRoot, "deb-root")
	debControlDir := filepath.Join(debRoot, "DEBIAN")

	for _, dir := range []string{
		buildRoot,
		filepath.Join(tarRoot, "usr", "bin"),
		filepath.Join(tarRoot, "lib", "systemd", "system"),
		filepath.Join(tarRoot, "etc", "vreflinkd"),
		filepath.Join(tarRoot, "share", "vreflink"),
		filepath.Join(debRoot, "usr", "bin"),
		filepath.Join(debRoot, "usr", "share", "vreflink"),
		filepath.Join(debRoot, "lib", "systemd", "system"),
		filepath.Join(debRoot, "etc", "vreflinkd"),
		debControlDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Artifacts{}, fmt.Errorf("create staging directory %s: %w", dir, err)
		}
	}

	buildEnv := append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
	if result, err := devsupport.RunCommand(ctx, repoRoot, buildEnv, "go", "build", "-o", filepath.Join(buildRoot, "vreflink"), "./cmd/vreflink"); err != nil {
		return Artifacts{}, fmt.Errorf("build vreflink: %w\n%s", err, strings.TrimSpace(result.Stderr))
	}
	if result, err := devsupport.RunCommand(ctx, repoRoot, buildEnv, "go", "build", "-o", filepath.Join(buildRoot, "vreflinkd"), "./cmd/vreflinkd"); err != nil {
		return Artifacts{}, fmt.Errorf("build vreflinkd: %w\n%s", err, strings.TrimSpace(result.Stderr))
	}

	if err := copyFile(filepath.Join(buildRoot, "vreflink"), filepath.Join(tarRoot, "usr", "bin", "vreflink"), 0o755); err != nil {
		return Artifacts{}, err
	}
	if err := copyFile(filepath.Join(buildRoot, "vreflinkd"), filepath.Join(tarRoot, "usr", "bin", "vreflinkd"), 0o755); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(tarRoot, "lib", "systemd", "system", "vreflinkd.service"), pkgassets.SystemdUnitTemplate(), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(tarRoot, "etc", "vreflinkd", "config.toml"), pkgassets.DaemonConfigTemplate(), 0o600); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(tarRoot, "share", "vreflink", "config.toml"), pkgassets.GuestConfigTemplate(), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := copyFile(filepath.Join(repoRoot, "README.md"), filepath.Join(tarRoot, "README.md"), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := copyFile(filepath.Join(repoRoot, "LICENSE"), filepath.Join(tarRoot, "LICENSE"), 0o644); err != nil {
		return Artifacts{}, err
	}

	if err := copyFile(filepath.Join(buildRoot, "vreflink"), filepath.Join(debRoot, "usr", "bin", "vreflink"), 0o755); err != nil {
		return Artifacts{}, err
	}
	if err := copyFile(filepath.Join(buildRoot, "vreflinkd"), filepath.Join(debRoot, "usr", "bin", "vreflinkd"), 0o755); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(debRoot, "lib", "systemd", "system", "vreflinkd.service"), pkgassets.SystemdUnitTemplate(), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(debRoot, "etc", "vreflinkd", "config.toml"), pkgassets.DaemonConfigTemplate(), 0o600); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(debRoot, "usr", "share", "vreflink", "config.toml"), pkgassets.GuestConfigTemplate(), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(debControlDir, "conffiles"), pkgassets.DebConffiles(), 0o644); err != nil {
		return Artifacts{}, err
	}

	control := string(pkgassets.DebControlTemplate())
	control = strings.ReplaceAll(control, "@VERSION@", opts.Version)
	control = strings.ReplaceAll(control, "@ARCH@", arch)
	if err := writeFile(filepath.Join(debControlDir, "control"), []byte(control), 0o644); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(debControlDir, "prerm"), pkgassets.DebPrerm(), 0o755); err != nil {
		return Artifacts{}, err
	}
	if err := writeFile(filepath.Join(debControlDir, "postrm"), pkgassets.DebPostrm(), 0o755); err != nil {
		return Artifacts{}, err
	}

	artifacts := Artifacts{
		TarballPath:   filepath.Join(outDir, tarRootName+".tar.gz"),
		DebPath:       filepath.Join(outDir, fmt.Sprintf("vreflink_%s_%s.deb", opts.Version, arch)),
		ChecksumsPath: filepath.Join(outDir, fmt.Sprintf("vreflink_%s_sha256sums.txt", opts.Version)),
	}

	if err := createTarGz(artifacts.TarballPath, tarRootName, tarRoot); err != nil {
		return Artifacts{}, err
	}

	if result, err := devsupport.RunCommand(ctx, repoRoot, nil, "dpkg-deb", "--root-owner-group", "--build", debRoot, artifacts.DebPath); err != nil {
		return Artifacts{}, fmt.Errorf("build deb package: %w\n%s", err, strings.TrimSpace(result.Stderr))
	}

	tarballHash, err := hashFile(artifacts.TarballPath)
	if err != nil {
		return Artifacts{}, err
	}
	debHash, err := hashFile(artifacts.DebPath)
	if err != nil {
		return Artifacts{}, err
	}
	checksums := fmt.Sprintf("%s  %s\n%s  %s\n", tarballHash, filepath.Base(artifacts.TarballPath), debHash, filepath.Base(artifacts.DebPath))
	if err := writeFile(artifacts.ChecksumsPath, []byte(checksums), 0o644); err != nil {
		return Artifacts{}, err
	}

	return artifacts, nil
}

var execLookPath = func(file string) (string, error) {
	if resolved, ok := devsupport.LookPathAny(file); ok {
		return resolved, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func createTarGz(dstPath, rootName, rootDir string) error {
	file, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create tarball %s: %w", dstPath, err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(rootName, relPath))
		if d.IsDir() {
			header.Name += "/"
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}

		_, err = io.Copy(tarWriter, src)
		closeErr := src.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

func copyFile(srcPath, dstPath string, mode fs.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcPath, err)
	}
	return writeFile(dstPath, data, mode)
}

func writeFile(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func ReadTarballEntries(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	var entries []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, header.Name)
	}

	return entries, nil
}

func VerifyChecksums(path string, targets ...string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != len(targets) {
		return fmt.Errorf("checksum file contains %d entries, want %d", len(lines), len(targets))
	}

	expected := make(map[string]string, len(lines))
	for _, line := range lines {
		parts := strings.Fields(string(line))
		if len(parts) != 2 {
			return fmt.Errorf("invalid checksum line %q", string(line))
		}
		expected[parts[1]] = parts[0]
	}

	for _, target := range targets {
		got, err := hashFile(target)
		if err != nil {
			return err
		}
		want, ok := expected[filepath.Base(target)]
		if !ok {
			return fmt.Errorf("checksum for %s not found", filepath.Base(target))
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s", filepath.Base(target))
		}
	}

	return nil
}
