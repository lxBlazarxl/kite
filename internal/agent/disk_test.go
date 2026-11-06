package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestSelectPartitions(t *testing.T) {
	in := []disk.PartitionStat{
		{Device: "/dev/nvme0n1p2", Mountpoint: "/var/lib/docker/overlay2/x", Fstype: "ext4"},
		{Device: "/dev/nvme0n1p2", Mountpoint: "/", Fstype: "ext4"},
		{Device: "/dev/nvme0n1p1", Mountpoint: "/boot/efi", Fstype: "vfat"},
		{Device: "/dev/sda1", Mountpoint: "/data", Fstype: "ext4"},
		{Device: "/dev/loop0", Mountpoint: "/snap/core/1", Fstype: "squashfs"},
		{Device: "efivarfs", Mountpoint: "/sys/firmware/efi/efivars", Fstype: "efivarfs"},
	}
	var got []string
	for _, p := range selectPartitions(in) {
		got = append(got, p.Mountpoint)
	}
	if want := []string{"/", "/boot/efi", "/data"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDiskReal(t *testing.T) {
	cfg := Config{Name: "pc", Interval: time.Second, Root: "/"}
	cols := []Collector{NewDisk()}
	CollectOnce(context.Background(), cfg, cols, time.Now())
	time.Sleep(1100 * time.Millisecond)
	s := CollectOnce(context.Background(), cfg, cols, time.Now())
	for _, d := range s.Disks {
		if d.Mount == "/" {
			if d.TotalBytes == 0 || d.ReadBps == nil {
				t.Fatalf("root disk incomplete: %+v", d)
			}
			return
		}
	}
	t.Fatalf("no / disk in %+v (unavailable %v)", s.Disks, s.Unavailable)
}

func TestIODeviceNameFollowsMapper(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "dm-0"), nil, 0o644)
	os.MkdirAll(filepath.Join(dir, "mapper"), 0o755)
	if err := os.Symlink("../dm-0", filepath.Join(dir, "mapper", "vg-root")); err != nil {
		t.Fatal(err)
	}
	if got := ioDeviceName(filepath.Join(dir, "mapper", "vg-root")); got != "dm-0" {
		t.Fatalf("got %q want dm-0", got)
	}
	if got := ioDeviceName("/dev/does-not-exist-sda1"); got != "does-not-exist-sda1" {
		t.Fatalf("plain device: got %q", got)
	}
}

func TestIODeviceNameDriveLetter(t *testing.T) {
	if got := ioDeviceName("C:"); got != "C:" {
		t.Errorf("C: -> %q", got)
	}
	if got := ioDeviceName(`D:\`); got != "D:" {
		t.Errorf(`D:\ -> %q`, got)
	}
}
