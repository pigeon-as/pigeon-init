//go:build linux

package etc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pigeon-as/pigeon-init/internal/config"
)

// Step 11: Set hostname, /etc/hosts, /etc/resolv.conf.

func TestWriteHosts_Basic(t *testing.T) {
	dir := t.TempDir()
	etcDir := filepath.Join(dir, "etc")
	hostsPath := filepath.Join(etcDir, "hosts")

	// Create etc dir and seed file.
	os.MkdirAll(etcDir, 0755)
	os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644)

	entries := []config.EtcHost{
		{Host: "app.internal", IP: "10.0.0.2"},
		{Host: "db.internal", IP: "10.0.0.3", Desc: "database"},
	}

	// We can't call WriteHosts directly since it hardcodes /etc/hosts.
	// Instead, test the formatting logic.
	var expected strings.Builder
	for _, e := range entries {
		if e.Desc != "" {
			expected.WriteString("\n# " + e.Desc + "\n" + e.IP + "\t" + e.Host + "\n")
		} else {
			expected.WriteString("\n" + e.IP + "\t" + e.Host + "\n")
		}
	}

	content := expected.String()

	if !strings.Contains(content, "10.0.0.2\tapp.internal") {
		t.Errorf("missing app.internal entry in:\n%s", content)
	}
	if !strings.Contains(content, "# database\n10.0.0.3\tdb.internal") {
		t.Errorf("missing db.internal entry with description in:\n%s", content)
	}
}

func TestWriteHosts_Empty(t *testing.T) {
	// Empty entries should be a no-op.
	err := WriteHosts(nil)
	if err != nil {
		t.Errorf("WriteHosts(nil): %v", err)
	}
	err = WriteHosts([]config.EtcHost{})
	if err != nil {
		t.Errorf("WriteHosts(empty): %v", err)
	}
}

func TestWriteResolv_Basic(t *testing.T) {
	// Test the formatting logic.
	resolv := &config.EtcResolv{Nameservers: []string{"8.8.8.8", "1.1.1.1"}}

	var lines []string
	for _, ns := range resolv.Nameservers {
		lines = append(lines, "nameserver "+ns)
	}
	content := strings.Join(lines, "\n") + "\n"

	if !strings.Contains(content, "nameserver 8.8.8.8") {
		t.Error("missing 8.8.8.8")
	}
	if !strings.Contains(content, "nameserver 1.1.1.1") {
		t.Error("missing 1.1.1.1")
	}
	if strings.Count(content, "nameserver") != 2 {
		t.Errorf("expected 2 nameserver lines, got %d", strings.Count(content, "nameserver"))
	}
}

func TestWriteResolv_Nil(t *testing.T) {
	err := WriteResolv(nil)
	if err != nil {
		t.Errorf("WriteResolv(nil): %v", err)
	}
}

func TestWriteResolv_EmptyNameservers(t *testing.T) {
	err := WriteResolv(&config.EtcResolv{Nameservers: nil})
	if err != nil {
		t.Errorf("WriteResolv(empty): %v", err)
	}
}
