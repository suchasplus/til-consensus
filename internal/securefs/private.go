package securefs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	PrivateDirMode  os.FileMode = 0o700
	PrivateFileMode os.FileMode = 0o600
)

// EnsurePrivateDir creates path with private permissions and rejects a final
// symlink. It does not chmod an existing caller-owned parent directory.
func EnsurePrivateDir(path string) error {
	path = filepath.Clean(path)
	if err := os.MkdirAll(path, PrivateDirMode); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink directory: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

// TightenPrivateDir is for directories owned by til-consensus itself, such as
// a run artifact root or session store root.
func TightenPrivateDir(path string) error {
	if err := EnsurePrivateDir(path); err != nil {
		return err
	}
	return os.Chmod(filepath.Clean(path), PrivateDirMode)
}

// EnsureContained verifies that target stays within root after lexical path
// normalization. Symlinks are checked separately at the actual I/O boundary.
func EnsureContained(root string, target string) error {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return err
	}
	if rel == ".." || filepath.IsAbs(rel) || len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("path escapes root: root=%s target=%s", rootAbs, targetAbs)
	}
	return nil
}

// ValidateRegularTarget rejects symlinks and non-regular existing targets.
func ValidateRegularTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink file: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path is not a regular file: %s", path)
	}
	return nil
}

func OpenRead(path string) (*os.File, error) {
	return openNoFollow(path, unix.O_RDONLY, 0)
}

func ReadFile(path string) ([]byte, error) {
	file, err := OpenRead(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func OpenAppend(path string) (*os.File, error) {
	if err := EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := openNoFollow(path, unix.O_CREAT|unix.O_WRONLY|unix.O_APPEND, uint32(PrivateFileMode.Perm()))
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(PrivateFileMode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func OpenLock(path string) (*os.File, error) {
	if err := EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := openNoFollow(path, unix.O_CREAT|unix.O_RDWR, uint32(PrivateFileMode.Perm()))
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(PrivateFileMode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func WriteFile(path string, body []byte) error {
	if err := EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := openNoFollow(path, unix.O_CREAT|unix.O_WRONLY|unix.O_TRUNC, uint32(PrivateFileMode.Perm()))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if err := file.Chmod(PrivateFileMode); err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		return err
	}
	return file.Sync()
}

// WriteFileAtomic durably replaces path with a private regular file. The file
// and its containing directory are synced before success is reported.
func WriteFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := EnsurePrivateDir(dir); err != nil {
		return err
	}
	if err := ValidateRegularTarget(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".til-consensus-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keepTemp := true
	defer func() {
		_ = tmp.Close()
		if keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(PrivateFileMode); err != nil {
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ValidateRegularTarget(path); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	keepTemp = false
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

func openNoFollow(path string, flags int, perm uint32) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, perm)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
