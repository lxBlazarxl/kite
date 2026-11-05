package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/retransmit/kite/internal/protocol"
)

const dockerConcurrency = 8

type cpuReading struct{ total, system uint64 }

type dockerCollector struct {
	client *http.Client
	base   string
	prev   map[string]cpuReading
}

// NewDocker reports container state, CPU and memory through the Docker
// Engine API. host is unix:///path/to/docker.sock or tcp://host:port.
func NewDocker(host string) (Collector, bool) {
	c := &dockerCollector{prev: map[string]cpuReading{}}
	switch {
	case strings.HasPrefix(host, "unix://"):
		sock := strings.TrimPrefix(host, "unix://")
		if _, err := os.Stat(sock); err != nil {
			return nil, false
		}
		c.base = "http://docker"
		c.client = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		}}
	case strings.HasPrefix(host, "tcp://"):
		c.base = "http://" + strings.TrimPrefix(host, "tcp://")
		c.client = &http.Client{}
	default:
		return nil, false
	}
	return c, true
}

func (c *dockerCollector) Name() string { return "docker" }

type dockerListEntry struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
	State string   `json:"State"`
}

func (c *dockerCollector) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker %s: %s", path, resp.Status)
	}
	return body, nil
}

type statResult struct {
	ok            bool
	total, system uint64
	cpus          int
	mem           uint64
}

func (c *dockerCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	body, err := c.get(ctx, "/containers/json?all=1")
	if err != nil {
		return err
	}
	var list []dockerListEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("docker list: %w", err)
	}

	results := make([]statResult, len(list))
	sem := make(chan struct{}, dockerConcurrency)
	var wg sync.WaitGroup
	for i, e := range list {
		if e.State != "running" {
			continue
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, err := c.get(ctx, "/containers/"+id+"/stats?stream=false&one-shot=true")
			if err != nil {
				return
			}
			total, system, cpus, mem, err := parseStats(b)
			if err != nil {
				return
			}
			results[i] = statResult{true, total, system, cpus, mem}
		}(i, e.ID)
	}
	wg.Wait()

	seen := map[string]bool{}
	d := &protocol.Docker{Containers: []protocol.Container{}}
	for i, e := range list {
		ct := protocol.Container{Image: e.Image, State: e.State}
		if len(e.Names) > 0 {
			ct.Name = strings.TrimPrefix(e.Names[0], "/")
		} else {
			ct.Name = e.ID[:min(12, len(e.ID))]
		}
		if r := results[i]; r.ok {
			seen[e.ID] = true
			mem := r.mem
			ct.MemBytes = &mem
			if p, had := c.prev[e.ID]; had && r.total >= p.total && r.system > p.system {
				pct := float64(r.total-p.total) / float64(r.system-p.system) * float64(r.cpus) * 100
				ct.CPUPct = &pct
			}
			c.prev[e.ID] = cpuReading{r.total, r.system}
		}
		d.Containers = append(d.Containers, ct)
	}
	for id := range c.prev {
		if !seen[id] {
			delete(c.prev, id)
		}
	}
	sort.Slice(d.Containers, func(i, j int) bool { return d.Containers[i].Name < d.Containers[j].Name })
	s.Docker = d
	return nil
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

// parseStats extracts CPU counters and working-set memory (usage minus
// inactive page cache, as `docker stats` shows) from a stats response.
func parseStats(b []byte) (cpuTotal, systemTotal uint64, onlineCPUs int, mem uint64, err error) {
	var st dockerStats
	if err = json.Unmarshal(b, &st); err != nil {
		return
	}
	cpuTotal = st.CPUStats.CPUUsage.TotalUsage
	systemTotal = st.CPUStats.SystemCPUUsage
	onlineCPUs = st.CPUStats.OnlineCPUs
	mem = st.MemoryStats.Usage
	inactive, ok := st.MemoryStats.Stats["inactive_file"]
	if !ok {
		inactive = st.MemoryStats.Stats["total_inactive_file"]
	}
	if inactive <= mem {
		mem -= inactive
	}
	return
}
