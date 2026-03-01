//go:build e2e

// Run with: sudo make e2e
// Or:       make testdata && sudo go test -tags=e2e -count=1 -v ./e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	log "github.com/sirupsen/logrus"

	"github.com/pigeon-as/pigeon-init/internal/config"
)

func TestMain(m *testing.M) {
	u, err := user.Current()
	if err != nil || u.Uid != "0" {
		fmt.Fprintln(os.Stderr, "e2e tests require root; skipping")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// retryBoot calls bootVM up to 3 times, retrying on timeout (nested KVM is flaky).
func retryBoot(t *testing.T, rc *config.RunConfig) string {
	t.Helper()
	for i := 0; i < 3; i++ {
		out, ok := bootVM(t, rc)
		if ok {
			return out
		}
		t.Logf("attempt %d: timeout; retrying", i+1)
	}
	t.Fatal("VM timed out 3 times")
	return ""
}

// bootVM launches a Firecracker micro-VM with the given RunConfig (delivered
// via MMDS), waits for it to exit, and returns the serial console output.
// Returns (output, false) on timeout so the caller can retry.
func bootVM(t *testing.T, rc *config.RunConfig) (string, bool) {
	t.Helper()

	for _, p := range []string{"testdata/vmlinux", "testdata/initrd.cpio", "testdata/rootfs.ext4"} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("testdata missing (%s) — run 'make testdata' first", p)
		}
	}

	// Each test gets its own rootfs copy so writes don't leak between tests.
	rootfs := filepath.Join(t.TempDir(), "rootfs.ext4")
	if err := cpFile("testdata/rootfs.ext4", rootfs); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "fc.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fcBin := "firecracker"
	if v := os.Getenv("FC_BIN"); v != "" {
		fcBin = v
	}

	var stdout bytes.Buffer
	cmd := firecracker.VMCommandBuilder{}.
		WithBin(fcBin).
		WithSocketPath(sock).
		WithStdout(&stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	m, err := firecracker.NewMachine(ctx, firecracker.Config{
		SocketPath:      sock,
		KernelImagePath: "testdata/vmlinux",
		InitrdPath:      "testdata/initrd.cpio",
		KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off acpi=off",
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(1),
			MemSizeMib: firecracker.Int64(256),
		},
		Drives: []models.Drive{{
			DriveID:      firecracker.String("rootfs"),
			PathOnHost:   &rootfs,
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(false),
		}},
		NetworkInterfaces: firecracker.NetworkInterfaces{{
			StaticConfiguration: &firecracker.StaticNetworkConfiguration{
				HostDevName: "tap0",
				MacAddress:  "02:FC:00:00:00:01",
			},
			AllowMMDS: true,
		}},
		MmdsVersion: firecracker.MMDSv2,
	}, firecracker.WithProcessRunner(cmd), firecracker.WithLogger(log.NewEntry(logger)))
	if err != nil {
		t.Fatalf("NewMachine: %v", err)
	}

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if rc != nil {
		b, _ := json.Marshal(rc)
		var md map[string]interface{}
		json.Unmarshal(b, &md)
		if err := m.SetMetadata(ctx, md); err != nil {
			t.Fatalf("SetMetadata: %v", err)
		}
	}

	waitErr := m.Wait(ctx)
	m.StopVMM()

	out := stdout.String()
	if waitErr != nil {
		return out, false
	}
	t.Logf("serial output (%d bytes):\n%s", len(out), out)
	return out, true
}

func cpFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// sh returns an ExecOverride that runs cmd via /bin/sh -c.
func sh(cmd string) []string { return []string{"/bin/sh", "-c", cmd} }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBoot_ExitZero(t *testing.T) {
	out := retryBoot(t, &config.RunConfig{ExecOverride: []string{"/bin/true"}})
	if !strings.Contains(out, `exit_code=0`) {
		t.Fatal("expected exit_code=0")
	}
}

func TestBoot_ExitOne(t *testing.T) {
	out := retryBoot(t, &config.RunConfig{ExecOverride: []string{"/bin/false"}})
	if !strings.Contains(out, `exit_code=1`) {
		t.Fatal("expected exit_code=1")
	}
}

func TestConfig_MMDS(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		Hostname:     "mmds-works",
		ExecOverride: sh(`test "$(hostname)" = "mmds-works"`),
	})
}

func TestMount_Essential(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		ExecOverride: sh(`test "$(cat /proc/1/comm)" = "init"`),
	})
}

func TestMount_Cgroups(t *testing.T) {
	retryBoot(t, &config.RunConfig{ExecOverride: sh("test -d /sys/fs/cgroup")})
}

func TestUser_Root(t *testing.T) {
	retryBoot(t, &config.RunConfig{ExecOverride: sh(`test "$(id -u)" = "0"`)})
}

func TestUser_Override(t *testing.T) {
	user := "nobody"
	out := retryBoot(t, &config.RunConfig{
		UserOverride: &user,
		ExecOverride: []string{"/bin/true"},
	})
	if !strings.Contains(out, `uid=65534`) {
		t.Fatal("expected uid=65534 (nobody)")
	}
}

func TestEnv_Extra(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		ExtraEnv:     map[string]string{"MY_VAR": "hello"},
		ExecOverride: sh(`test "$MY_VAR" = "hello"`),
	})
}

func TestEnv_ImageEnv(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		ImageConfig: &config.ImageConfig{
			Entrypoint: []string{"/bin/sh"},
			Cmd:        []string{"-c", `test "$IMG_VAR" = "from_image"`},
			Env:        []string{"IMG_VAR=from_image"},
		},
	})
}

func TestEtc_Hostname(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		Hostname:     "pigeon-test",
		ExecOverride: sh(`test "$(hostname)" = "pigeon-test"`),
	})
}

func TestEtc_Hosts(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		EtcHosts:     []config.EtcHost{{Host: "myhost.test", IP: "10.0.0.99"}},
		ExecOverride: sh(`grep -q "10.0.0.99.*myhost.test" /etc/hosts`),
	})
}

func TestEtc_Resolv(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		EtcResolv:    &config.EtcResolv{Nameservers: []string{"8.8.8.8"}},
		ExecOverride: sh(`grep -q "nameserver 8.8.8.8" /etc/resolv.conf`),
	})
}

func TestNet_Loopback(t *testing.T) {
	retryBoot(t, &config.RunConfig{ExecOverride: sh("ip addr show lo | grep -q 127.0.0.1")})
}

func TestWorkDir(t *testing.T) {
	retryBoot(t, &config.RunConfig{
		ImageConfig:  &config.ImageConfig{WorkingDir: "/tmp"},
		ExecOverride: sh(`test "$(pwd)" = "/tmp"`),
	})
}
