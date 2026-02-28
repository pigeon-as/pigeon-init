//go:build linux

package etc

import (
	"testing"

	"github.com/pigeon-as/pigeon-init/internal/config"
)

func TestWriteHosts_NilIsNoop(t *testing.T) {
	if err := WriteHosts(nil); err != nil {
		t.Errorf("WriteHosts(nil): %v", err)
	}
}

func TestWriteHosts_EmptyIsNoop(t *testing.T) {
	if err := WriteHosts([]config.EtcHost{}); err != nil {
		t.Errorf("WriteHosts(empty): %v", err)
	}
}

func TestWriteResolv_NilIsNoop(t *testing.T) {
	if err := WriteResolv(nil); err != nil {
		t.Errorf("WriteResolv(nil): %v", err)
	}
}

func TestWriteResolv_EmptyNameserversIsNoop(t *testing.T) {
	if err := WriteResolv(&config.EtcResolv{Nameservers: nil}); err != nil {
		t.Errorf("WriteResolv(empty): %v", err)
	}
}
