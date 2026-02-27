# pigeon-init

PID 1 init tailored for Firecracker micro-VMs.

> **Experimental.** This init code is modeled after the [Fly.io init](https://github.com/superfly/init-snapshot), since it's a good reference for Firecracker based init programs.

## What It Does

`pigeon-init` runs as PID 1 inside a Firecracker micro-VM. It handles the full guest lifecycle:

1. **Mount devtmpfs** + redirect console to `/dev/ttyS0`
2. **Load config** — MMDS (`169.254.169.254`) primary, `/pigeon/run.json` fallback
3. **Mount rootfs + switch_root** — mounts root device (default `/dev/vda`), pivots into it
4. **Mount essential filesystems** — `/proc`, `/sys`, `/dev/pts`, `/dev/shm`, `/dev/mqueue`, `/dev/hugepages`, `/run`, `/proc/sys/fs/binfmt_misc`
5. **Mount cgroups** — v1 + v2 hybrid (10 v1 controllers + unified cgroupv2)
6. **Set rlimits** — NOFILE to 10240
7. **Resolve user/group** — from image config or override (`/etc/passwd` + `/etc/group`)
8. **Build env** — merge image env + extra env, set PATH
9. **Start vsock API** — HTTP on vsock port 10000 (comes up early so host can probe readiness)
10. **Mount extra volumes** — additional block device mounts with chown
11. **Set hostname, /etc/hosts, /etc/resolv.conf**
12. **Configure networking** — lo up, eth0 MTU + up, disable checksums, add addresses (IFA_F_NODAD), add routes
13. **Spawn workload** — fork/exec with credentials, setsid, merged stdout/stderr pipe
14. **Main loop** — SIGCHLD-driven reaping, signal forwarding to process group
15. **Shutdown** — OOM check, unmount (retry + lazy fallback), sync, reboot

## Build

```bash
make build      # Static init binary → out/init
make initrd     # Build initrd cpio (depends on build)
make rootfs     # Docker image → ext4 rootfs
make test       # Run unit tests
make clean      # Remove build artifacts
```

Requires Go 1.23+. `initrd` needs cpio + bash, `rootfs` needs Docker.

## Configuration

Config is delivered via [Firecracker MMDS](https://github.com/firecracker-microvm/firecracker/blob/main/docs/mmds/mmds-user-guide.md) (V2 token + V1 fallback). If MMDS is unreachable, falls back to `/pigeon/run.json` baked into the initrd.

The JSON format is the contract between the host driver and guest init. PascalCase field names follow the Fly.io convention.

```json
{
  "ImageConfig": {
    "Entrypoint": ["/bin/myapp"],
    "Cmd": ["--port", "8080"],
    "Env": ["PATH=/usr/local/bin:/usr/bin:/bin"],
    "WorkingDir": "/app",
    "User": "nobody"
  },
  "ExecOverride": null,
  "CmdOverride": null,
  "UserOverride": null,
  "ExtraEnv": {
    "DATABASE_URL": "postgres://...",
    "LOG_LEVEL": "info"
  },
  "MTU": 1500,
  "IPConfigs": [
    {
      "Gateway": "169.254.0.1",
      "IP": "10.0.0.2",
      "Mask": 24
    }
  ],
  "Hostname": "my-app-abcdef",
  "Mounts": [
    {
      "DevicePath": "/dev/vdb",
      "MountPath": "/data"
    }
  ],
  "RootDevice": null,
  "EtcResolv": {
    "Nameservers": ["8.8.8.8"]
  },
  "EtcHosts": [
    {"Host": "my-app.internal", "IP": "10.0.0.2"}
  ]
}
```

### Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ImageConfig` | object | — | OCI image metadata (entrypoint, cmd, env, user, workdir) |
| `ExecOverride` | string[] | — | Replaces the entire argv (highest priority) |
| `CmdOverride` | string | — | Replaces the Cmd portion of argv |
| `UserOverride` | string | — | Overrides the image user (`"user"` or `"user:group"`) |
| `ExtraEnv` | map | — | Merged on top of `ImageConfig.Env` |
| `MTU` | int | 1500 | MTU for eth0 |
| `IPConfigs` | array | — | Network addresses and routes for eth0 (omit to skip networking) |
| `Hostname` | string | — | Guest hostname (omit to skip) |
| `Mounts` | array | — | Extra block device mounts |
| `RootDevice` | string | `/dev/vda` | Root filesystem device path |
| `EtcResolv` | object | — | `/etc/resolv.conf` nameservers (omit to skip) |
| `EtcHosts` | array | — | Entries appended to `/etc/hosts` (omit to skip) |

### Argv Resolution

Priority order:
1. `ExecOverride` — replaces everything
2. `ImageConfig.Entrypoint` + `CmdOverride` — entrypoint with overridden cmd
3. `ImageConfig.Entrypoint` + `ImageConfig.Cmd` — default OCI behavior

## Vsock API

HTTP/1.1 on vsock port 10000 (any CID).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/status` | Health check (`{"ok": true}`) |
| `GET` | `/v1/exit_code` | Blocks until workload exits (`{"code": N, "oom_killed": bool}`) |
| `POST` | `/v1/signals` | Send signal to workload (`{"signal": 15}`) |
| `POST` | `/v1/exec` | One-shot command (`{"cmd": ["ls", "-la"]}`) |
| `GET` | `/v1/ws/exec` | WebSocket interactive exec (optional PTY) |

The vsock API becoming reachable is the implicit readiness signal.