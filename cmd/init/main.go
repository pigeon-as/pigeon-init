// PID 1 init for Firecracker micro-VMs.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/pigeon-as/pigeon-init/internal/api"
	"github.com/pigeon-as/pigeon-init/internal/boot"
	"github.com/pigeon-as/pigeon-init/internal/config"
	"github.com/pigeon-as/pigeon-init/internal/etc"
	"github.com/pigeon-as/pigeon-init/internal/netcfg"
	"github.com/pigeon-as/pigeon-init/internal/process"
	"github.com/pigeon-as/pigeon-init/internal/shutdown"
	"github.com/pigeon-as/pigeon-init/internal/user"
)

const configPath = "/pigeon/run.json"
const mmdsTimeout = 3 * time.Second

func main() {
	if err := boot.MountDev(); err != nil {
		fatal("mount dev", err)
	}

	setupConsole()

	level := slog.LevelInfo
	if os.Getenv("INIT_LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("pigeon-init starting")

	cfg, err := loadConfig(logger)
	if err != nil {
		fatal("load config", err)
	}

	if err := boot.MountRootfs(cfg.RootDev()); err != nil {
		fatal("mount rootfs", err)
	}
	if err := boot.MoveDev(); err != nil {
		fatal("move dev", err)
	}
	boot.RemoveConfig()
	if err := boot.SwitchRoot(); err != nil {
		fatal("switch root", err)
	}

	if err := boot.MountEssential(); err != nil {
		fatal("mount essential", err)
	}
	if err := boot.MountCgroups(logger); err != nil {
		fatal("mount cgroups", err)
	}

	if err := boot.SetRlimits(); err != nil {
		logger.Warn("set rlimits failed", "err", err)
	}

	userSpec := "root"
	if cfg.UserOverride != nil {
		userSpec = *cfg.UserOverride
	} else if cfg.ImageConfig != nil && cfg.ImageConfig.User != "" {
		userSpec = cfg.ImageConfig.User
	}
	identity, err := user.Resolve(userSpec)
	if err != nil {
		fatal("resolve user", err)
	}
	logger.Info("resolved user", "uid", identity.UID, "gid", identity.GID, "home", identity.HomeDir)

	var imageEntrypoint, imageCmd, imageEnv []string
	var workDir string
	if cfg.ImageConfig != nil {
		imageEntrypoint = cfg.ImageConfig.Entrypoint
		imageCmd = cfg.ImageConfig.Cmd
		imageEnv = cfg.ImageConfig.Env
		workDir = cfg.ImageConfig.WorkingDir
	}

	env := api.BuildEnv(imageEnv, cfg.ExtraEnv, identity.HomeDir)

	for _, e := range env {
		if len(e) > 5 && e[:5] == "PATH=" {
			os.Setenv("PATH", e[5:])
			break
		}
	}

	argv := api.BuildArgv(cfg.ExecOverride, imageEntrypoint, imageCmd, cfg.CmdOverride)
	if len(argv) == 0 {
		fatal("empty argv: no command configured", nil)
	}

	sup, err := process.New(argv, env, workDir, identity, logger)
	if err != nil {
		fatal("create supervisor", err)
	}

	apiServer := api.NewServer(sup, env, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := apiServer.Serve(ctx); err != nil {
			logger.Warn("vsock API error", "err", err)
		}
	}()

	if err := shutdown.MountExtra(cfg.Mounts, identity.UID, identity.GID, logger); err != nil {
		fatal("mount extra", err)
	}

	if err := etc.SetHostname(cfg.Hostname); err != nil {
		logger.Warn("set hostname failed", "err", err)
	}
	if err := etc.WriteHosts(cfg.EtcHosts); err != nil {
		logger.Warn("write /etc/hosts failed", "err", err)
	}
	if err := etc.WriteResolv(cfg.EtcResolv); err != nil {
		logger.Warn("write /etc/resolv.conf failed", "err", err)
	}

	if err := netcfg.Configure(cfg.IPConfigs, cfg.MTU); err != nil {
		fatal("configure network", err)
	}

	if err := sup.Start(); err != nil {
		fatal("start workload", err)
	}

	result := sup.Run()

	logger.Info("workload exited", "exit_code", result.ExitCode, "oom_killed", result.OOMKilled)
	shutdown.Shutdown(cfg.Mounts, logger)
	cancel()
}

func loadConfig(logger *slog.Logger) (*config.RunConfig, error) {
	if err := netcfg.SetupMMDS(); err != nil {
		logger.Debug("mmds network setup failed, using config file", "err", err)
		return config.Load(configPath)
	}
	defer netcfg.CleanupMMDS()

	ctx, cancel := context.WithTimeout(context.Background(), mmdsTimeout)
	defer cancel()

	cfg, err := config.FetchMMDS(ctx)
	if err != nil {
		logger.Debug("mmds fetch failed, using config file", "err", err)
		return config.Load(configPath)
	}

	logger.Info("config loaded from MMDS")
	return cfg, nil
}

func setupConsole() {
	fd, err := unix.Open("/dev/ttyS0", unix.O_RDWR, 0)
	if err != nil {
		return
	}
	for _, target := range []int{0, 1, 2} {
		_ = unix.Dup3(fd, target, 0)
	}
	if fd > 2 {
		unix.Close(fd)
	}
}

func fatal(msg string, err error) {
	if err != nil {
		slog.Error(msg, "err", err)
	} else {
		slog.Error(msg)
	}
	_ = unix.Reboot(unix.LINUX_REBOOT_CMD_RESTART)
	os.Exit(1)
}
