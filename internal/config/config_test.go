package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.json")

	cfg := RunConfig{
		Hostname: "test-vm",
		MTU:      1400,
		ImageConfig: &ImageConfig{
			Entrypoint: []string{"/bin/app"},
			Cmd:        []string{"serve"},
			Env:        []string{"PATH=/usr/bin"},
			WorkingDir: "/app",
			User:       "nobody",
		},
		ExtraEnv: map[string]string{"LOG_LEVEL": "debug"},
		IPConfigs: []IPConfig{
			{Gateway: "10.0.0.1", IP: "10.0.0.2", Mask: 24},
		},
		Mounts: []Mount{
			{DevicePath: "/dev/vdb", MountPath: "/data"},
		},
		EtcResolv: &EtcResolv{Nameservers: []string{"8.8.8.8"}},
		EtcHosts: []EtcHost{
			{Host: "app.internal", IP: "10.0.0.2", Desc: "app"},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Hostname != "test-vm" {
		t.Errorf("Hostname: got %q, want test-vm", loaded.Hostname)
	}
	if loaded.MTU != 1400 {
		t.Errorf("MTU: got %d, want 1400", loaded.MTU)
	}
	if loaded.ImageConfig == nil {
		t.Fatal("ImageConfig is nil")
	}
	if len(loaded.ImageConfig.Entrypoint) != 1 || loaded.ImageConfig.Entrypoint[0] != "/bin/app" {
		t.Errorf("Entrypoint: got %v", loaded.ImageConfig.Entrypoint)
	}
	if len(loaded.IPConfigs) != 1 || loaded.IPConfigs[0].IP != "10.0.0.2" {
		t.Errorf("IPConfigs: got %v", loaded.IPConfigs)
	}
	if len(loaded.EtcHosts) != 1 || loaded.EtcHosts[0].Desc != "app" {
		t.Errorf("EtcHosts: got %v", loaded.EtcHosts)
	}
}

func TestLoad_MinimalConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.json")

	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if cfg.ImageConfig != nil {
		t.Errorf("ImageConfig should be nil for empty JSON")
	}
	if cfg.RootDev() != "/dev/vda" {
		t.Errorf("RootDev default: got %q, want /dev/vda", cfg.RootDev())
	}
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load("/nonexistent/path.json")
	if err == nil {
		t.Error("Load nonexistent: expected error")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("Load invalid JSON: expected error")
	}
}

func TestRootDev_Default(t *testing.T) {
	cfg := &RunConfig{}
	if got := cfg.RootDev(); got != "/dev/vda" {
		t.Errorf("RootDev default: got %q, want /dev/vda", got)
	}
}

func TestRootDev_Override(t *testing.T) {
	dev := "/dev/vdb"
	cfg := &RunConfig{RootDevice: &dev}
	if got := cfg.RootDev(); got != "/dev/vdb" {
		t.Errorf("RootDev override: got %q, want /dev/vdb", got)
	}
}

func TestRootDev_EmptyStringFallsBack(t *testing.T) {
	empty := ""
	cfg := &RunConfig{RootDevice: &empty}
	if got := cfg.RootDev(); got != "/dev/vda" {
		t.Errorf("RootDev empty string: got %q, want /dev/vda", got)
	}
}

func TestRunConfig_JSONRoundTrip(t *testing.T) {
	override := "run"
	user := "app:app"
	root := "/dev/vdc"
	original := RunConfig{
		ImageConfig: &ImageConfig{
			Entrypoint: []string{"/bin/server"},
			Cmd:        []string{"--port", "8080"},
			Env:        []string{"PATH=/bin"},
			WorkingDir: "/srv",
			User:       "nobody",
		},
		ExecOverride: []string{"/bin/custom"},
		CmdOverride:  &override,
		UserOverride: &user,
		ExtraEnv:     map[string]string{"KEY": "val"},
		MTU:          1420,
		IPConfigs:    []IPConfig{{Gateway: "10.0.0.1", IP: "10.0.0.2", Mask: 24}},
		Hostname:     "test",
		Mounts:       []Mount{{DevicePath: "/dev/vdb", MountPath: "/data"}},
		RootDevice:   &root,
		EtcResolv:    &EtcResolv{Nameservers: []string{"1.1.1.1", "8.8.8.8"}},
		EtcHosts:     []EtcHost{{Host: "db", IP: "10.0.0.3"}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded RunConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Hostname != original.Hostname {
		t.Errorf("Hostname: got %q, want %q", decoded.Hostname, original.Hostname)
	}
	if decoded.MTU != original.MTU {
		t.Errorf("MTU: got %d, want %d", decoded.MTU, original.MTU)
	}
	if decoded.CmdOverride == nil || *decoded.CmdOverride != "run" {
		t.Errorf("CmdOverride: got %v", decoded.CmdOverride)
	}
	if decoded.RootDev() != "/dev/vdc" {
		t.Errorf("RootDev: got %q", decoded.RootDev())
	}
	if len(decoded.EtcResolv.Nameservers) != 2 {
		t.Errorf("Nameservers count: got %d, want 2", len(decoded.EtcResolv.Nameservers))
	}
}

func TestRunConfig_PascalCaseFieldNames(t *testing.T) {
	data := []byte(`{
		"ImageConfig": {"Entrypoint": ["/bin/sh"], "Cmd": ["-c", "echo hi"]},
		"ExecOverride": ["/bin/custom"],
		"CmdOverride": "test",
		"UserOverride": "nobody",
		"ExtraEnv": {"A": "B"},
		"MTU": 9000,
		"IPConfigs": [{"Gateway": "10.0.0.1", "IP": "10.0.0.2", "Mask": 24}],
		"Hostname": "vm-1",
		"Mounts": [{"DevicePath": "/dev/vdb", "MountPath": "/data"}],
		"RootDevice": "/dev/vdc",
		"EtcResolv": {"Nameservers": ["8.8.8.8"]},
		"EtcHosts": [{"Host": "db", "IP": "10.0.0.3", "Desc": "database"}]
	}`)

	var cfg RunConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal PascalCase: %v", err)
	}
	if cfg.Hostname != "vm-1" {
		t.Errorf("Hostname: got %q", cfg.Hostname)
	}
	if cfg.MTU != 9000 {
		t.Errorf("MTU: got %d", cfg.MTU)
	}
	if cfg.CmdOverride == nil || *cfg.CmdOverride != "test" {
		t.Errorf("CmdOverride: got %v", cfg.CmdOverride)
	}
	if len(cfg.EtcHosts) != 1 || cfg.EtcHosts[0].Desc != "database" {
		t.Errorf("EtcHosts: got %v", cfg.EtcHosts)
	}
}
