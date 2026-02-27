package etc

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/pigeon-as/pigeon-init/internal/config"
)

func SetHostname(hostname string) error {
	if hostname == "" {
		return nil
	}
	if err := unix.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}
	_ = os.MkdirAll("/etc", 0755)
	if err := os.WriteFile("/etc/hostname", []byte(hostname+"\n"), 0644); err != nil {
		return fmt.Errorf("write /etc/hostname: %w", err)
	}
	return nil
}

func WriteHosts(entries []config.EtcHost) error {
	if len(entries) == 0 {
		return nil
	}
	_ = os.MkdirAll("/etc", 0755)
	f, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open /etc/hosts: %w", err)
	}
	defer f.Close()

	for _, e := range entries {
		var err error
		if e.Desc != "" {
			_, err = fmt.Fprintf(f, "\n# %s\n%s\t%s\n", e.Desc, e.IP, e.Host)
		} else {
			_, err = fmt.Fprintf(f, "\n%s\t%s\n", e.IP, e.Host)
		}
		if err != nil {
			return fmt.Errorf("write /etc/hosts: %w", err)
		}
	}
	return nil
}

func WriteResolv(resolv *config.EtcResolv) error {
	if resolv == nil || len(resolv.Nameservers) == 0 {
		return nil
	}
	_ = os.MkdirAll("/etc", 0755)
	var lines []string
	for _, ns := range resolv.Nameservers {
		lines = append(lines, "nameserver "+ns)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile("/etc/resolv.conf", []byte(content), 0644); err != nil {
		return fmt.Errorf("write /etc/resolv.conf: %w", err)
	}
	return nil
}
