package agent

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/shirou/gopsutil/v4/disk"
)

var skipFSTypes = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "overlay": true, "squashfs": true,
	"efivarfs": true, "iso9660": true, "ramfs": true,
}

var skipMountPrefixes = []string{"/var/lib/docker/", "/snap/", "/run/"}

// selectPartitions keeps real filesystems, one entry per device (the
// shortest mountpoint, so bind mounts collapse), sorted by mountpoint.
func selectPartitions(parts []disk.PartitionStat) []disk.PartitionStat {
	byDev := map[string]disk.PartitionStat{}
	for _, p := range parts {
		if skipFSTypes[p.Fstype] {
			continue
		}
		skip := false
		for _, pre := range skipMountPrefixes {
			if strings.HasPrefix(p.Mountpoint, pre) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if cur, ok := byDev[p.Device]; !ok || len(p.Mountpoint) < len(cur.Mountpoint) {
			byDev[p.Device] = p
		}
	}
	out := make([]disk.PartitionStat, 0, len(byDev))
	for _, p := range byDev {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mountpoint < out[j].Mountpoint })
	return out
}

type diskCollector struct{ rates *Rates }

// NewDisk reports usage and read/write throughput per filesystem.
func NewDisk() Collector { return &diskCollector{rates: NewRates()} }

func (c *diskCollector) Name() string { return "disk" }

func (c *diskCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return err
	}
	io, _ := disk.IOCountersWithContext(ctx)
	for _, p := range selectPartitions(parts) {
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}
		d := protocol.Disk{Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype, TotalBytes: u.Total, UsedBytes: u.Used}
		dev := ioDeviceName(p.Device)
		if st, ok := io[dev]; ok {
			if v, ok := c.rates.Rate("disk."+dev+".r", st.ReadBytes, s.Time); ok {
				d.ReadBps = &v
			}
			if v, ok := c.rates.Rate("disk."+dev+".w", st.WriteBytes, s.Time); ok {
				d.WriteBps = &v
			}
		}
		s.Disks = append(s.Disks, d)
	}
	return nil
}

// ioDeviceName maps a mounted device to its /proc/diskstats name, following
// symlinks such as /dev/mapper/vg-root -> /dev/dm-0 (LVM, LUKS).
func ioDeviceName(device string) string {
	// Windows drive letters ("C:", `D:\`) are already the IO counter key;
	// EvalSymlinks("C:") would resolve to the current directory on that drive.
	if len(device) >= 2 && device[1] == ':' && len(strings.TrimRight(device, `\/`)) == 2 {
		return strings.ToUpper(device[:2])
	}
	if real, err := filepath.EvalSymlinks(device); err == nil {
		device = real
	}
	return filepath.Base(device)
}
