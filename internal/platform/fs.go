// Package platform wraps the Linux filesystem primitives that make Siphon
// crash-safe: same-directory temps, fsync, renameat2, sparse probes, and
// magic-byte validation.
package platform

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/siphonerr"
)

// RandomHex returns n hex characters derived from crypto/rand.
func RandomHex(n int) (string, error) {
	if n%2 != 0 {
		n++
	}
	b := make([]byte, n/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// TempPath builds a hidden same-directory name:
// .siphon-<stem>-<8hex><suffix>
func TempPath(dir, stem, suffix string) (string, error) {
	tag, err := RandomHex(8)
	if err != nil {
		return "", err
	}
	stem = sanitizeStem(stem)
	return filepath.Join(dir, fmt.Sprintf(".siphon-%s-%s%s", stem, tag, suffix)), nil
}

// CreateTemp creates a same-directory temp file with O_EXCL|O_NOFOLLOW, mode
// 0600. The caller owns the returned file and must remove it on failure.
func CreateTemp(dir, stem, suffix string) (*os.File, error) {
	for attempt := 0; attempt < 100; attempt++ {
		path, err := TempPath(dir, stem, suffix)
		if err != nil {
			return nil, err
		}
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err == unix.EEXIST {
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		return os.NewFile(uintptr(fd), path), nil
	}
	return nil, siphonerr.New(siphonerr.Internal, "could not create a unique temp file in %s", dir)
}

func sanitizeStem(stem string) string {
	stem = filepath.Base(stem)
	stem = strings.Map(func(r rune) rune {
		switch r {
		case '/', 0:
			return '_'
		}
		return r
	}, stem)
	if stem == "" || stem == "." {
		return "file"
	}
	if len(stem) > 80 {
		stem = stem[:80]
	}
	return stem
}

// Fsync flushes a file's data and metadata.
func Fsync(f *os.File) error { return f.Sync() }

// Fdatasync flushes file data and only the metadata needed to retrieve it.
// The reclaim cursor protocol uses it for its periodic checkpoints (spec
// §4.4.1) so timestamp churn does not turn every checkpoint into a journal
// commit for the whole inode.
func Fdatasync(f *os.File) error {
	if err := unix.Fdatasync(int(f.Fd())); err != nil {
		return &os.PathError{Op: "fdatasync", Path: f.Name(), Err: err}
	}
	return nil
}

// FsyncPath opens path O_RDONLY|O_NOFOLLOW and fsyncs it. Used to make a temp
// durable before rename without keeping a write descriptor around.
func FsyncPath(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return &os.PathError{Op: "open-fsync", Path: path, Err: err}
	}
	defer unix.Close(fd)
	if err := unix.Fsync(fd); err != nil {
		return &os.PathError{Op: "fsync", Path: path, Err: err}
	}
	return nil
}

// WriteFile replaces the contents of an existing file without following a
// symlink. The caller owns the path and its lifetime.
func WriteFile(path string, data []byte) error {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return &os.PathError{Op: "open-write", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return &os.PathError{Op: "write", Path: path, Err: err}
	}
	if err := f.Close(); err != nil {
		return &os.PathError{Op: "close", Path: path, Err: err}
	}
	return nil
}

// LockFile takes a non-blocking exclusive advisory lock on an open file. It
// marks a live same-directory temp so the reaper can tell an abandoned temp
// from one an active conversion still owns.
func LockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return &os.PathError{Op: "flock", Path: f.Name(), Err: err}
	}
	return nil
}

// ReapTemps removes same-directory temps for stem that are older than minAge
// and are not locked by a live process. Locked temps (see LockFile) are left
// alone, so a concurrent conversion is never disturbed.
func ReapTemps(dir, stem string, minAge time.Duration) ([]string, error) {
	prefix := ".siphon-" + sanitizeStem(stem) + "-"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var removed []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.Contains(name, ".tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < minAge {
			continue
		}
		path := filepath.Join(dir, name)
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
			unix.Close(fd) // still owned by a live conversion
			continue
		}
		if err := unix.Unlink(path); err == nil {
			removed = append(removed, path)
		}
		unix.Close(fd)
	}
	return removed, nil
}

// FsyncDir fsyncs a directory so a rename or unlink is durable.
func FsyncDir(dir string) error {
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return &os.PathError{Op: "open-dir", Path: dir, Err: err}
	}
	defer unix.Close(fd)
	if err := unix.Fsync(fd); err != nil {
		return &os.PathError{Op: "fsync-dir", Path: dir, Err: err}
	}
	return nil
}

// RenameNoReplace atomically moves old to new, failing with EEXIST if new
// exists. This is the only finalisation primitive used by Siphon.
func RenameNoReplace(oldPath, newPath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	return &os.LinkError{Op: "renameat2", Old: oldPath, New: newPath, Err: err}
}

// RenameReplace atomically moves old to new, replacing an existing target.
// Used only with --force, after the pre-flight confirmation.
func RenameReplace(oldPath, newPath string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, 0); err != nil {
		return &os.LinkError{Op: "renameat2", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

// SameDevice reports whether two paths live on the same filesystem.
func SameDevice(a, b string) (bool, error) {
	var sa, sb unix.Stat_t
	if err := unix.Stat(a, &sa); err != nil {
		return false, err
	}
	if err := unix.Stat(b, &sb); err != nil {
		return false, err
	}
	return sa.Dev == sb.Dev, nil
}

// FreeSpace returns available bytes (non-root view) for the filesystem
// containing path.
func FreeSpace(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// SparseProbe verifies that the filesystem can represent a large sparse file.
// It creates a temp in dir, ftruncates it to size, and requires allocated
// blocks to stay below size/10.
func SparseProbe(dir string, size int64) error {
	if size <= 0 {
		size = 16 << 20
	}
	f, err := CreateTemp(dir, "sparse-probe", ".tmp")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	defer f.Close()

	if err := unix.Ftruncate(int(f.Fd()), size); err != nil {
		return err
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return err
	}
	allocated := st.Blocks * 512
	if allocated >= size/10 {
		return siphonerr.New(siphonerr.FSNoSparse,
			"%s does not support sparse files (allocated %d of %d bytes)", dir, allocated, size)
	}
	return nil
}

// OpenSource opens a source O_RDWR|O_NOFOLLOW for the destructive engines.
func OpenSource(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open-source", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

// OpenReadOnly opens a source O_RDONLY|O_NOFOLLOW.
func OpenReadOnly(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open-read", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

// IsRegularNoSymlink validates the source path without following a symlink.
func IsRegularNoSymlink(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, siphonerr.New(siphonerr.SourceInvalid, "cannot stat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, siphonerr.New(siphonerr.SourceInvalid, "%s is a symlink; Siphon refuses to follow it", path)
	}
	if !info.Mode().IsRegular() {
		return nil, siphonerr.New(siphonerr.SourceInvalid, "%s is not a regular file", path)
	}
	return info, nil
}

// IsWritable reports whether the effective user may write to path.
func IsWritable(path string) error {
	if err := unix.Access(path, unix.W_OK); err != nil {
		return siphonerr.New(siphonerr.Permission, "%s is not writable: %v", path, err)
	}
	return nil
}

// IsDirWritable reports whether the effective user may create entries in dir.
func IsDirWritable(dir string) error {
	if err := unix.Access(dir, unix.W_OK|unix.X_OK); err != nil {
		return siphonerr.New(siphonerr.Permission, "directory %s is not writable: %v", dir, err)
	}
	return nil
}

// RemoveIfExists is a quiet cleanup helper used by deferred paths.
func RemoveIfExists(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Cleanup is best-effort; callers report the primary failure.
		_ = err
	}
}

// Snapshot is the identity of a source file at a point in time. Engines
// re-take it before any destructive step so a concurrent editor is caught.
type Snapshot struct {
	Dev     uint64
	Ino     uint64
	Size    int64
	MtimeNS int64
	CtimeNS int64
}

// TakeSnapshot stats path without following symlinks.
func TakeSnapshot(path string) (Snapshot, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return Snapshot{}, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot stat %s", path)
	}
	return Snapshot{
		Dev:     uint64(st.Dev),
		Ino:     st.Ino,
		Size:    st.Size,
		MtimeNS: st.Mtim.Nano(),
		CtimeNS: st.Ctim.Nano(),
	}, nil
}

// SameIdentity reports whether the file at path is still the same inode.
func (s Snapshot) SameIdentity(path string) error {
	got, err := TakeSnapshot(path)
	if err != nil {
		return err
	}
	if got.Dev != s.Dev || got.Ino != s.Ino {
		return siphonerr.New(siphonerr.SourceChanged, "%s was replaced while it was being converted", path)
	}
	return nil
}

// Check re-takes the snapshot and requires the file to be byte-identical in
// size and mtime. Safe-mode engines call it right before finalisation.
func (s Snapshot) Check(path string) error {
	got, err := TakeSnapshot(path)
	if err != nil {
		return err
	}
	if got != s {
		return siphonerr.New(siphonerr.SourceChanged,
			"%s changed while it was being converted; nothing was replaced", path)
	}
	return nil
}

// CheckOpen verifies that an already-open descriptor still matches the
// snapshot, closing the window between the path check and the open.
func (s Snapshot) CheckOpen(f *os.File) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot stat the open source")
	}
	got := Snapshot{
		Dev:     uint64(st.Dev),
		Ino:     st.Ino,
		Size:    st.Size,
		MtimeNS: st.Mtim.Nano(),
		CtimeNS: st.Ctim.Nano(),
	}
	if got != s {
		return siphonerr.New(siphonerr.SourceChanged,
			"%s changed while it was being opened", f.Name())
	}
	return nil
}
