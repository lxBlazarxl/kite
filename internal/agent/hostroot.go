package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// applyHostRoot points gopsutil at the host's filesystems when the agent
// runs in a container with the host's / mounted at root. Variables the
// user already set are left alone.
func applyHostRoot(root string, getenv func(string) string, setenv func(k, v string) error) {
	for k, sub := range map[string]string{
		"HOST_PROC": "proc", "HOST_SYS": "sys", "HOST_ETC": "etc", "HOST_VAR": "var",
		"HOST_RUN": "run", "HOST_DEV": "dev", "HOST_ROOT": "",
	} {
		if getenv(k) == "" {
			setenv(k, filepath.Join(root, sub))
		}
	}
}

// hostHostname reads the host's name from <root>/etc/hostname, so a
// containerised agent reports the host rather than its container ID.
func hostHostname(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "etc", "hostname"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

// usagePath is where a host mountpoint can be read from inside the container.
func usagePath(hostRoot, mount string) string {
	if hostRoot == "" {
		return mount
	}
	return filepath.Join(hostRoot, mount)
}
