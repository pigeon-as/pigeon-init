package boot

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const newroot = "/newroot"

func MountDev() error {
	if err := os.MkdirAll("/dev", 0755); err != nil {
		return err
	}
	return unix.Mount("devtmpfs", "/dev", "devtmpfs", unix.MS_NOSUID, "mode=0755")
}

func MountRootfs(device string) error {
	if err := os.MkdirAll(newroot, 0755); err != nil {
		return err
	}
	return unix.Mount(device, newroot, "ext4", unix.MS_RELATIME, "")
}

func MoveDev() error {
	dst := filepath.Join(newroot, "dev")
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return unix.Mount("/dev", dst, "", unix.MS_MOVE, "")
}

func RemoveConfig() {
	_ = os.RemoveAll("/pigeon")
}

// SwitchRoot: chdir → MS_MOVE → chroot → chdir.
func SwitchRoot() error {
	if err := unix.Chdir(newroot); err != nil {
		return fmt.Errorf("chdir %s: %w", newroot, err)
	}
	if err := unix.Mount(".", "/", "", unix.MS_MOVE, ""); err != nil {
		return fmt.Errorf("MS_MOVE . → /: %w", err)
	}
	if err := unix.Chroot("."); err != nil {
		return fmt.Errorf("chroot: %w", err)
	}
	if err := unix.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}
	return nil
}

func MountEssential() error {
	mounts := []struct {
		source string
		target string
		fstype string
		flags  uintptr
		data   string
		mode   os.FileMode
	}{
		{"devpts", "/dev/pts", "devpts", unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NOATIME, "mode=0620,gid=5,ptmxmode=666", 0755},
		{"mqueue", "/dev/mqueue", "mqueue", unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_NOSUID, "", 0755},
		{"tmpfs", "/dev/shm", "tmpfs", unix.MS_NOSUID | unix.MS_NODEV, "", 01777},
		{"hugetlbfs", "/dev/hugepages", "hugetlbfs", unix.MS_RELATIME, "pagesize=2M", 0755},
		{"proc", "/proc", "proc", unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_NOSUID, "", 0555},
		{"binfmt_misc", "/proc/sys/fs/binfmt_misc", "binfmt_misc", unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_RELATIME, "", 0555},
		{"sysfs", "/sys", "sysfs", unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_NOSUID, "", 0555},
		{"tmpfs", "/run", "tmpfs", unix.MS_NOSUID | unix.MS_NODEV, "mode=0755", 0755},
	}

	for _, m := range mounts {
		if err := os.MkdirAll(m.target, m.mode); err != nil {
			return fmt.Errorf("mkdir %s: %w", m.target, err)
		}
		if err := unix.Mount(m.source, m.target, m.fstype, m.flags, m.data); err != nil {
			return fmt.Errorf("mount %s → %s: %w", m.fstype, m.target, err)
		}
	}

	if err := os.MkdirAll("/run/lock", 01777); err != nil {
		return fmt.Errorf("mkdir /run/lock: %w", err)
	}

	if err := os.MkdirAll("/root", 0700); err != nil {
		return fmt.Errorf("mkdir /root: %w", err)
	}

	symlinks := [][2]string{
		{"/proc/self/fd", "/dev/fd"},
		{"/proc/self/fd/0", "/dev/stdin"},
		{"/proc/self/fd/1", "/dev/stdout"},
		{"/proc/self/fd/2", "/dev/stderr"},
	}
	for _, sl := range symlinks {
		_ = os.Remove(sl[1])
		if err := os.Symlink(sl[0], sl[1]); err != nil {
			return fmt.Errorf("symlink %s → %s: %w", sl[0], sl[1], err)
		}
	}

	return nil
}

func MountCgroups(logger *slog.Logger) error {
	base := "/sys/fs/cgroup"
	flags := uintptr(unix.MS_NODEV | unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_RELATIME)

	if err := os.MkdirAll(base, 0555); err != nil {
		return err
	}
	if err := unix.Mount("tmpfs", base, "tmpfs", unix.MS_NOSUID|unix.MS_NOEXEC|unix.MS_NODEV, "mode=755"); err != nil {
		return fmt.Errorf("mount cgroup tmpfs: %w", err)
	}

	unified := filepath.Join(base, "unified")
	if err := os.MkdirAll(unified, 0555); err != nil {
		return err
	}
	if err := unix.Mount("cgroup2", unified, "cgroup2", flags, "nsdelegate"); err != nil {
		return fmt.Errorf("mount cgroup2: %w", err)
	}

	controllers := []string{
		"net_cls,net_prio",
		"hugetlb",
		"pids",
		"freezer",
		"cpu,cpuacct",
		"devices",
		"blkio",
		"memory",
		"perf_event",
		"cpuset",
	}
	for _, ctrl := range controllers {
		dir := filepath.Join(base, ctrl)
		if err := os.MkdirAll(dir, 0555); err != nil {
			return err
		}
		if err := unix.Mount("cgroup", dir, "cgroup", flags, ctrl); err != nil {
			logger.Warn("cgroup controller mount failed", "controller", ctrl, "err", err)
			continue
		}
	}

	return nil
}

func SetRlimits() error {
	limit := &unix.Rlimit{Cur: 10240, Max: 10240}
	return unix.Setrlimit(unix.RLIMIT_NOFILE, limit)
}
