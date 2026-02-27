package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type RunConfig struct {
	ImageConfig  *ImageConfig      `json:"ImageConfig,omitempty"`
	ExecOverride []string           `json:"ExecOverride,omitempty"`
	CmdOverride  *string            `json:"CmdOverride,omitempty"`
	UserOverride *string            `json:"UserOverride,omitempty"`
	ExtraEnv     map[string]string  `json:"ExtraEnv,omitempty"`
	IPConfigs    []IPConfig         `json:"IPConfigs,omitempty"`
	MTU          int                `json:"MTU,omitempty"`
	Hostname     string             `json:"Hostname,omitempty"`
	Mounts       []Mount            `json:"Mounts,omitempty"`
	RootDevice   *string            `json:"RootDevice,omitempty"`
	EtcResolv    *EtcResolv         `json:"EtcResolv,omitempty"`
	EtcHosts     []EtcHost          `json:"EtcHosts,omitempty"`
}

type ImageConfig struct {
	Entrypoint []string `json:"Entrypoint,omitempty"`
	Cmd        []string `json:"Cmd,omitempty"`
	Env        []string `json:"Env,omitempty"`
	WorkingDir string   `json:"WorkingDir,omitempty"`
	User       string   `json:"User,omitempty"`
}

type IPConfig struct {
	Gateway string `json:"Gateway"`
	IP      string `json:"IP"`
	Mask    int    `json:"Mask"`
}

type Mount struct {
	DevicePath string `json:"DevicePath"`
	MountPath  string `json:"MountPath"`
}

type EtcResolv struct {
	Nameservers []string `json:"Nameservers,omitempty"`
}

type EtcHost struct {
	Host string `json:"Host"`
	IP   string `json:"IP"`
	Desc string `json:"Desc,omitempty"`
}

func Load(path string) (*RunConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg RunConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func (c *RunConfig) RootDev() string {
	if c.RootDevice != nil && *c.RootDevice != "" {
		return *c.RootDevice
	}
	return "/dev/vda"
}
