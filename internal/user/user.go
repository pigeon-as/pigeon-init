package user

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Identity struct {
	UID     uint32
	GID     uint32
	HomeDir string
}

func Resolve(spec string) (*Identity, error) {
	if spec == "" {
		spec = "root"
	}

	userPart, groupPart := spec, ""
	if i := strings.Index(spec, ":"); i >= 0 {
		userPart = spec[:i]
		groupPart = spec[i+1:]
	}

	uid, gid, home, err := lookupUser(userPart)
	if err != nil {
		return nil, fmt.Errorf("resolve user %q: %w", userPart, err)
	}

	if groupPart != "" {
		resolvedGID, err := lookupGroup(groupPart)
		if err != nil {
			return nil, fmt.Errorf("resolve group %q: %w", groupPart, err)
		}
		gid = resolvedGID
	}

	return &Identity{UID: uid, GID: gid, HomeDir: home}, nil
}

func lookupUser(name string) (uint32, uint32, string, error) {
	if entry, ok := findPasswdByName(name); ok {
		return entry.uid, entry.gid, entry.home, nil
	}

	if uid, err := strconv.ParseUint(name, 10, 32); err == nil {
		if entry, ok := findPasswdByUID(uint32(uid)); ok {
			return entry.uid, entry.gid, entry.home, nil
		}
		return uint32(uid), uint32(uid), "/", nil
	}

	if name == "root" {
		return 0, 0, "/root", nil
	}

	return 0, 0, "", fmt.Errorf("user %q not found in /etc/passwd", name)
}

func lookupGroup(name string) (uint32, error) {
	if gid, err := strconv.ParseUint(name, 10, 32); err == nil {
		return uint32(gid), nil
	}
	gid, ok := findGroupByName(name)
	if !ok {
		return 0, fmt.Errorf("group %q not found in /etc/group", name)
	}
	return gid, nil
}

type passwdEntry struct {
	uid  uint32
	gid  uint32
	home string
}

func findPasswdByName(name string) (passwdEntry, bool) {
	return scanPasswd(func(fields []string) bool { return fields[0] == name })
}

func findPasswdByUID(uid uint32) (passwdEntry, bool) {
	return scanPasswd(func(fields []string) bool {
		u, err := strconv.ParseUint(fields[2], 10, 32)
		return err == nil && uint32(u) == uid
	})
}

func scanPasswd(match func([]string) bool) (passwdEntry, bool) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return passwdEntry{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 6 {
			continue
		}
		if match(fields) {
			uid, _ := strconv.ParseUint(fields[2], 10, 32)
			gid, _ := strconv.ParseUint(fields[3], 10, 32)
			home := fields[5]
			return passwdEntry{uid: uint32(uid), gid: uint32(gid), home: home}, true
		}
	}
	return passwdEntry{}, false
}

func findGroupByName(name string) (uint32, bool) {
	f, err := os.Open("/etc/group")
	if err != nil {
		return 0, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 3 {
			continue
		}
		if fields[0] == name {
			gid, err := strconv.ParseUint(fields[2], 10, 32)
			if err != nil {
				continue
			}
			return uint32(gid), true
		}
	}
	return 0, false
}
