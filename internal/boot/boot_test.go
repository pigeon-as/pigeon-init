//go:build linux

package boot

import (
	"testing"
)

// Step 1/3: Verify newroot constant (used by MountRootfs, MoveDev, SwitchRoot).
func TestNewroot(t *testing.T) {
	if newroot != "/newroot" {
		t.Errorf("newroot: got %q, want /newroot", newroot)
	}
}

