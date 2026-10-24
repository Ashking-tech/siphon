package platform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/siphonerr"
)

// CopyMetadata transfers mode, ownership (when root), xattrs/ACLs, SELinux
// label, and atime from srcPath onto dstPath. mtime is not copied: the output
// records completion time (spec §6.5).
//
// security.selinux failures and capability-bearing sources are fatal; other
// xattr failures are returned as warnings.
func CopyMetadata(srcPath, dstPath string) (warnings []string, err error) {
	var st unix.Stat_t
	if err := unix.Stat(srcPath, &st); err != nil {
		return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot stat source metadata")
	}

	if err := unix.Chmod(dstPath, st.Mode&0o7777); err != nil {
		return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot copy file mode")
	}
	if os.Geteuid() == 0 {
		if err := unix.Chown(dstPath, int(st.Uid), int(st.Gid)); err != nil {
			return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot copy ownership")
		}
	}

	names, err := listXattrs(srcPath)
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			return nil, nil
		}
		return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot list extended attributes")
	}

	for _, name := range names {
		if name == "security.capability" {
			return nil, siphonerr.New(siphonerr.MetadataDrop,
				"source carries security.capability; refusing to replace it (is this really a media file?)")
		}
		value, err := getXattr(srcPath, name)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot read xattr %q", name)
		}
		// Temp files usually inherit the same SELinux label already. Skip
		// identical values: unprivileged users cannot set security.selinux.
		if current, err := getXattr(dstPath, name); err == nil && bytes.Equal(current, value) {
			continue
		}
		if err := unix.Setxattr(dstPath, name, value, 0); err != nil {
			switch {
			case name == "security.selinux":
				return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot restore SELinux label")
			case len(name) >= 9 && name[:9] == "security.":
				return nil, siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot restore %q", name)
			default:
				warnings = append(warnings, fmt.Sprintf("could not copy xattr %q: %v", name, err))
			}
		}
	}

	// Preserve atime; mtime becomes completion time.
	now := unix.NsecToTimespec(time.Now().UnixNano())
	times := []unix.Timespec{st.Atim, now}
	if err := unix.UtimesNanoAt(unix.AT_FDCWD, dstPath, times, 0); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not set atime: %v", err))
	}
	return warnings, nil
}

func listXattrs(path string) ([]string, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	n, err := unix.Listxattr(path, buf)
	if err != nil {
		return nil, err
	}
	var out []string
	start := 0
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			if i > start {
				out = append(out, string(buf[start:i]))
			}
			start = i + 1
		}
	}
	return out, nil
}

func getXattr(path, name string) ([]byte, error) {
	size, err := unix.Getxattr(path, name, nil)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	n, err := unix.Getxattr(path, name, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// HasImmutable reports whether the file carries FS_IMMUTABLE_FL.
func HasImmutable(path string) (bool, error) {
	f, err := OpenReadOnly(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	flags, err := unix.IoctlGetInt(int(f.Fd()), unix.FS_IOC_GETFLAGS)
	if err != nil {
		switch err {
		case unix.ENOTTY, unix.EINVAL, unix.ENOTSUP:
			return false, nil
		}
		return false, err
	}
	const fsImmutableFlag = 0x00000010
	return flags&fsImmutableFlag != 0, nil
}
