//go:build linux

package api

import (
	"sort"
	"testing"
)

func TestBuildArgv_ExecOverride(t *testing.T) {
	got := BuildArgv([]string{"/bin/custom", "--flag"}, []string{"/bin/entry"}, []string{"default"}, nil)
	want := []string{"/bin/custom", "--flag"}
	if !sliceEqual(got, want) {
		t.Errorf("BuildArgv exec override: got %v, want %v", got, want)
	}
}

func TestBuildArgv_EntrypointPlusCmd(t *testing.T) {
	got := BuildArgv(nil, []string{"/bin/entry"}, []string{"arg1", "arg2"}, nil)
	want := []string{"/bin/entry", "arg1", "arg2"}
	if !sliceEqual(got, want) {
		t.Errorf("BuildArgv entrypoint+cmd: got %v, want %v", got, want)
	}
}

func TestBuildArgv_EntrypointPlusCmdOverride(t *testing.T) {
	override := "overridden"
	got := BuildArgv(nil, []string{"/bin/entry"}, []string{"default"}, &override)
	want := []string{"/bin/entry", "overridden"}
	if !sliceEqual(got, want) {
		t.Errorf("BuildArgv entrypoint+cmdOverride: got %v, want %v", got, want)
	}
}

func TestBuildArgv_Empty(t *testing.T) {
	got := BuildArgv(nil, nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("BuildArgv empty: got %v, want empty", got)
	}
}

func TestBuildArgv_ExecOverrideBeatsAll(t *testing.T) {
	override := "cmd-override"
	got := BuildArgv(
		[]string{"/exec"},
		[]string{"/entry"},
		[]string{"cmd"},
		&override,
	)
	want := []string{"/exec"}
	if !sliceEqual(got, want) {
		t.Errorf("BuildArgv exec override priority: got %v, want %v", got, want)
	}
}

func TestBuildEnv_MergesImageAndExtra(t *testing.T) {
	got := BuildEnv(
		[]string{"PATH=/usr/bin", "FOO=bar"},
		map[string]string{"BAZ": "qux"},
		"/home/test",
	)
	env := envToMap(got)

	if env["PATH"] != "/usr/bin" {
		t.Errorf("PATH: got %q, want /usr/bin", env["PATH"])
	}
	if env["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want bar", env["FOO"])
	}
	if env["BAZ"] != "qux" {
		t.Errorf("BAZ: got %q, want qux", env["BAZ"])
	}
	if env["HOME"] != "/home/test" {
		t.Errorf("HOME: got %q, want /home/test", env["HOME"])
	}
}

func TestBuildEnv_ExtraOverridesImage(t *testing.T) {
	got := BuildEnv(
		[]string{"FOO=original"},
		map[string]string{"FOO": "overridden"},
		"/root",
	)
	env := envToMap(got)
	if env["FOO"] != "overridden" {
		t.Errorf("extra should override image: got %q, want overridden", env["FOO"])
	}
}

func TestBuildEnv_HomeNotOverridden(t *testing.T) {
	got := BuildEnv(
		[]string{"HOME=/custom"},
		nil,
		"/default",
	)
	env := envToMap(got)
	if env["HOME"] != "/custom" {
		t.Errorf("HOME should not be overridden: got %q, want /custom", env["HOME"])
	}
}

func TestBuildEnv_HomeDefaulted(t *testing.T) {
	got := BuildEnv(nil, nil, "/fallback")
	env := envToMap(got)
	if env["HOME"] != "/fallback" {
		t.Errorf("HOME default: got %q, want /fallback", env["HOME"])
	}
}

func TestBuildEnv_MalformedImageEnv(t *testing.T) {
	got := BuildEnv(
		[]string{"NOEQUALS", "GOOD=value", "=empty_key"},
		nil,
		"/root",
	)
	env := envToMap(got)
	if _, ok := env["NOEQUALS"]; ok {
		t.Error("malformed env var (no =) should be skipped")
	}
	if env["GOOD"] != "value" {
		t.Errorf("GOOD: got %q, want value", env["GOOD"])
	}
	if env[""] != "empty_key" {
		t.Errorf("empty key should be preserved: got %q", env[""])
	}
}

func TestBuildEnv_ValueWithEquals(t *testing.T) {
	got := BuildEnv([]string{"DSN=postgres://host?opt=val"}, nil, "/root")
	env := envToMap(got)
	if env["DSN"] != "postgres://host?opt=val" {
		t.Errorf("value with equals: got %q", env["DSN"])
	}
}

func TestParseEnvVar(t *testing.T) {
	tests := []struct {
		input string
		key   string
		val   string
		ok    bool
	}{
		{"FOO=bar", "FOO", "bar", true},
		{"PATH=/usr/bin:/bin", "PATH", "/usr/bin:/bin", true},
		{"EMPTY=", "EMPTY", "", true},
		{"=val", "", "val", true},
		{"NOEQUALS", "", "", false},
		{"", "", "", false},
		{"A=B=C", "A", "B=C", true},
	}
	for _, tt := range tests {
		k, v, ok := parseEnvVar(tt.input)
		if ok != tt.ok || k != tt.key || v != tt.val {
			t.Errorf("parseEnvVar(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.input, k, v, ok, tt.key, tt.val, tt.ok)
		}
	}
}

// helpers

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, e := range env {
		k, v, ok := parseEnvVar(e)
		if ok {
			m[k] = v
		}
	}
	return m
}

func sortedSlice(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	sort.Strings(c)
	return c
}
