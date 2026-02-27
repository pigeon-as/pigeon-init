package shutdown

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/pigeon-as/pigeon-init/internal/config"
)

func Shutdown(mounts []config.Mount, logger *slog.Logger) {
	for i := len(mounts) - 1; i >= 0; i-- {
		unmountWithRetry(mounts[i].MountPath, logger)
	}

	unix.Sync()

	time.Sleep(1 * time.Second)

	// Reboot terminates the VM in Firecracker.
	logger.Info("rebooting")
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
}

func unmountWithRetry(path string, logger *slog.Logger) {
	for i := 0; i < 5; i++ {
		if err := unix.Unmount(path, 0); err == nil {
			return
		}
		time.Sleep(750 * time.Millisecond)
	}

	// Lazy unmount fallback.
	logger.Warn("lazy unmount", "path", path)
	_ = unix.Unmount(path, unix.MNT_DETACH)
	unix.Sync()
}

func MountExtra(mounts []config.Mount, uid, gid uint32, logger *slog.Logger) error {
	for _, m := range mounts {
		if err := os.MkdirAll(m.MountPath, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", m.MountPath, err)
		}
		if err := unix.Mount(m.DevicePath, m.MountPath, "ext4", unix.MS_RELATIME, ""); err != nil {
			return fmt.Errorf("mount %s on %s: %w", m.DevicePath, m.MountPath, err)
		}
		if err := unix.Chown(m.MountPath, int(uid), int(gid)); err != nil {
			logger.Warn("chown mount failed", "path", m.MountPath, "err", err)
		}
	}
	return nil
}
