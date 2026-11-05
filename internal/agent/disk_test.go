package agent

import (
	"context"
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
