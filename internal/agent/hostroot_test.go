package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestApplyHostRoot(t *testing.T) {
	set := map[string]string{}
	preset := map[string]string{"HOST_PROC": "/custom/proc"}
	applyHostRoot("/host", func(k string) string { return preset[k] }, func(k, v string) error { set[k] = v; return nil })
	want := map[string]string{
		"HOST_SYS": "/host/sys", "HOST_ETC": "/host/etc", "HOST_VAR": "/host/var",
		"HOST_RUN": "/host/run", "HOST_DEV": "/host/dev", "HOST_ROOT": "/host",
	}
	for k, v := range want {
		if set[k] != v {
			t.Errorf("%s = %q, want %q", k, set[k], v)
		}
	}
	if _, ok := set["HOST_PROC"]; ok {
		t.Error("a preset HOST_PROC must not be overwritten")
	}
}

func TestHostRootDefaultName(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "etc"), 0o755)
	os.WriteFile(filepath.Join(root, "etc", "hostname"), []byte("server-01\n"), 0o644)
	t.Cleanup(func() {
		for _, k := range []string{"HOST_PROC", "HOST_SYS", "HOST_ETC", "HOST_VAR", "HOST_RUN", "HOST_DEV", "HOST_ROOT"} {
			os.Unsetenv(k)
		}
	})
	c, err := LoadConfig([]string{"-once"}, env(map[string]string{"KITE_HOST_ROOT": root}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "server-01" || c.Root != root || c.HostRoot != root {
		t.Fatalf("got name %q root %q hostroot %q", c.Name, c.Root, c.HostRoot)
	}
}

func TestHostRootMountpoints(t *testing.T) {
	if got := usagePath("/host", "/data"); got != "/host/data" {
		t.Errorf("usagePath with root: %q", got)
	}
	if got := usagePath("", "/data"); got != "/data" {
		t.Errorf("usagePath without root: %q", got)
	}
	in := []disk.PartitionStat{
		{Device: "/dev/sda1", Mountpoint: "/", Fstype: "ext4"},
		{Device: "/dev/sda1", Mountpoint: "/host", Fstype: "ext4"},
		{Device: "/dev/sdb1", Mountpoint: "/host/data", Fstype: "ext4"},
		{Device: "/dev/sdc1", Mountpoint: "/data", Fstype: "ext4"},
	}
	var got []string
	for _, p := range selectPartitions(in, "/host") {
		got = append(got, p.Mountpoint)
	}
	if len(got) != 2 || got[0] != "/" || got[1] != "/data" {
		t.Fatalf("got %v", got)
	}
}
