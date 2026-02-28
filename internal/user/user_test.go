package user

import (
	"os"
	"path/filepath"
	"testing"
)

// withPasswdGroup sets up temp /etc/passwd and /etc/group content for testing.
// It overrides the root filesystem paths used by the scanner functions.
// Since scanPasswd/findGroupByName hardcode /etc/passwd and /etc/group,
// we test the exported Resolve function indirectly through lookupUser/lookupGroup.

func TestLookupUser_ByName(t *testing.T) {
	writeTestPasswd(t, "app:x:1000:1000:App User:/home/app:/bin/sh\nroot:x:0:0:Root:/root:/bin/bash\n")

	uid, gid, home, err := lookupUser("app")
	if err != nil {
		t.Fatalf("lookupUser(app): %v", err)
	}
	if uid != 1000 || gid != 1000 || home != "/home/app" {
		t.Errorf("lookupUser(app): uid=%d gid=%d home=%q", uid, gid, home)
	}
}

func TestLookupUser_ByUID(t *testing.T) {
	writeTestPasswd(t, "app:x:1000:1000:App User:/home/app:/bin/sh\n")

	uid, gid, home, err := lookupUser("1000")
	if err != nil {
		t.Fatalf("lookupUser(1000): %v", err)
	}
	if uid != 1000 || gid != 1000 || home != "/home/app" {
		t.Errorf("lookupUser(1000): uid=%d gid=%d home=%q", uid, gid, home)
	}
}

func TestLookupUser_NumericNotInPasswd(t *testing.T) {
	writeTestPasswd(t, "root:x:0:0:Root:/root:/bin/bash\n")

	uid, gid, home, err := lookupUser("9999")
	if err != nil {
		t.Fatalf("lookupUser(9999): %v", err)
	}
	// Numeric UID not in passwd → uid=gid, home="/"
	if uid != 9999 || gid != 9999 || home != "/" {
		t.Errorf("lookupUser(9999): uid=%d gid=%d home=%q", uid, gid, home)
	}
}

func TestLookupUser_Root(t *testing.T) {
	// Write empty passwd — root fallback should still work.
	writeTestPasswd(t, "")

	uid, gid, home, err := lookupUser("root")
	if err != nil {
		t.Fatalf("lookupUser(root): %v", err)
	}
	if uid != 0 || gid != 0 || home != "/root" {
		t.Errorf("lookupUser(root): uid=%d gid=%d home=%q", uid, gid, home)
	}
}

func TestLookupUser_NotFound(t *testing.T) {
	writeTestPasswd(t, "root:x:0:0:Root:/root:/bin/bash\n")

	_, _, _, err := lookupUser("nonexistent")
	if err == nil {
		t.Error("lookupUser(nonexistent): expected error")
	}
}

func TestLookupGroup_ByName(t *testing.T) {
	writeTestGroup(t, "docker:x:999:\nstaff:x:50:app\n")

	gid, err := lookupGroup("docker")
	if err != nil {
		t.Fatalf("lookupGroup(docker): %v", err)
	}
	if gid != 999 {
		t.Errorf("lookupGroup(docker): gid=%d, want 999", gid)
	}
}

func TestLookupGroup_ByNumeric(t *testing.T) {
	gid, err := lookupGroup("42")
	if err != nil {
		t.Fatalf("lookupGroup(42): %v", err)
	}
	if gid != 42 {
		t.Errorf("lookupGroup(42): gid=%d, want 42", gid)
	}
}

func TestLookupGroup_NotFound(t *testing.T) {
	writeTestGroup(t, "root:x:0:\n")

	_, err := lookupGroup("nonexistent")
	if err == nil {
		t.Error("lookupGroup(nonexistent): expected error")
	}
}

func TestScanPasswd_SkipsComments(t *testing.T) {
	writeTestPasswd(t, "# comment\n\napp:x:1000:1000::/home/app:/bin/sh\n")

	entry, ok := findPasswdByName("app")
	if !ok {
		t.Fatal("findPasswdByName(app): not found")
	}
	if entry.uid != 1000 {
		t.Errorf("uid: got %d, want 1000", entry.uid)
	}
}

func TestScanPasswd_MalformedLine(t *testing.T) {
	writeTestPasswd(t, "short:line\napp:x:1000:1000::/home/app:/bin/sh\n")

	entry, ok := findPasswdByName("app")
	if !ok {
		t.Fatal("findPasswdByName(app): not found after malformed line")
	}
	if entry.uid != 1000 {
		t.Errorf("uid: got %d, want 1000", entry.uid)
	}
}

func TestScanPasswd_BadUID(t *testing.T) {
	// UID field is not numeric — should be skipped.
	writeTestPasswd(t, "baduid:x:abc:0::/root:/bin/bash\napp:x:1000:1000::/home/app:/bin/sh\n")

	entry, ok := findPasswdByName("baduid")
	if ok {
		t.Errorf("findPasswdByName(baduid): should not find entry with non-numeric UID, got %+v", entry)
	}
}

// writeTestPasswd writes content to /etc/passwd (or skips if not root).
func writeTestPasswd(t *testing.T, content string) {
	t.Helper()
	writeTestFile(t, "/etc/passwd", content)
}

func writeTestGroup(t *testing.T, content string) {
	t.Helper()
	writeTestFile(t, "/etc/group", content)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	// These tests require writing to /etc — only possible as root on Linux.
	// In CI they run inside a container; locally they'll skip.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Skipf("cannot create %s (need root): %v", dir, err)
	}

	// Back up existing file if present.
	orig, origErr := os.ReadFile(path)
	if origErr == nil {
		t.Cleanup(func() {
			os.WriteFile(path, orig, 0644)
		})
	} else {
		t.Cleanup(func() {
			os.Remove(path)
		})
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Skipf("cannot write %s (need root): %v", path, err)
	}
}
