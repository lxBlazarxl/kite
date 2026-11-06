package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

const containersJSON = `[
 {"Id":"aaa","Names":["/proxy"],"Image":"example/proxy","State":"running"},
 {"Id":"bbb","Names":["/store"],"Image":"x","State":"running"},
 {"Id":"ccc","Names":["/old"],"Image":"y","State":"exited"}]`

func statsJSON(total, system uint64) string {
	return fmt.Sprintf(`{"cpu_stats":{"cpu_usage":{"total_usage":%d},"system_cpu_usage":%d,"online_cpus":4},
 "memory_stats":{"usage":500000000,"stats":{"inactive_file":100000000}}}`, total, system)
}

type fakeDocker struct {
	mu       sync.Mutex
	aaaCalls int
	paths    []string
}

func (f *fakeDocker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, r.URL.Path)
	switch {
	case r.URL.Path == "/containers/json":
		fmt.Fprint(w, containersJSON)
	case r.URL.Path == "/containers/aaa/stats":
		if r.URL.Query().Get("one-shot") != "true" || r.URL.Query().Get("stream") != "false" {
			http.Error(w, "want one-shot", 400)
			return
		}
		f.aaaCalls++
		if f.aaaCalls == 1 {
			fmt.Fprint(w, statsJSON(1e9, 100e9))
		} else {
			fmt.Fprint(w, statsJSON(2e9, 110e9))
		}
	default:
		http.NotFound(w, r)
	}
}

func dockerSample(t *testing.T, c Collector) *protocol.Sample {
	t.Helper()
	s := &protocol.Sample{Time: time.Unix(100, 0)}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s
}

func byName(s *protocol.Sample, name string) protocol.Container {
	for _, c := range s.Docker.Containers {
		if c.Name == name {
			return c
		}
	}
	return protocol.Container{}
}

func TestDockerCPUAndMem(t *testing.T) {
	fd := &fakeDocker{}
	srv := httptest.NewServer(fd)
	defer srv.Close()
	c, ok := NewDocker("tcp://" + srv.Listener.Addr().String())
	if !ok {
		t.Fatal("tcp host must count as present")
	}
	s := dockerSample(t, c)
	var names []string
	for _, ct := range s.Docker.Containers {
		names = append(names, ct.Name)
	}
	if strings.Join(names, ",") != "old,proxy,store" {
		t.Fatalf("names %v", names)
	}
	if p := byName(s, "proxy"); p.CPUPct != nil || p.MemBytes == nil || *p.MemBytes != 400e6 || p.Image != "example/proxy" {
		t.Fatalf("first proxy %+v", p)
	}
	s = dockerSample(t, c)
	if p := byName(s, "proxy"); p.CPUPct == nil || *p.CPUPct != 40 {
		t.Fatalf("second proxy cpu %v", p.CPUPct)
	}
	if o := byName(s, "old"); o.State != "exited" || o.CPUPct != nil || o.MemBytes != nil {
		t.Fatalf("old %+v", o)
	}
	for _, p := range fd.paths {
		if strings.Contains(p, "ccc") {
			t.Fatal("stats requested for a stopped container")
		}
	}
}

func TestDockerContainerVanishes(t *testing.T) {
	srv := httptest.NewServer(&fakeDocker{})
	defer srv.Close()
	c, _ := NewDocker("tcp://" + srv.Listener.Addr().String())
	s := dockerSample(t, c)
	if p := byName(s, "store"); p.State != "running" || p.CPUPct != nil || p.MemBytes != nil {
		t.Fatalf("store %+v", p)
	}
}

func TestDockerDaemonDown(t *testing.T) {
	srv := httptest.NewServer(&fakeDocker{})
	addr := srv.Listener.Addr().String()
	srv.Close()
	c, _ := NewDocker("tcp://" + addr)
	s := &protocol.Sample{Time: time.Unix(1, 0)}
	if err := c.Collect(context.Background(), s); err == nil || s.Docker != nil {
		t.Fatalf("err %v docker %+v", err, s.Docker)
	}
}

func TestDockerUnixSocketMissing(t *testing.T) {
	if _, ok := NewDocker("unix://" + t.TempDir() + "/nope.sock"); ok {
		t.Fatal("missing socket must not be detected")
	}
}

func TestParseStatsReal(t *testing.T) {
	b, err := os.ReadFile("testdata/docker/stats-real.json")
	if err != nil {
		t.Fatal(err)
	}
	total, system, cpus, mem, err := parseStats(b)
	if err != nil || mem == 0 || cpus == 0 || total == 0 || system == 0 {
		t.Fatalf("total %d system %d cpus %d mem %d err %v", total, system, cpus, mem, err)
	}
}

func TestNpipePath(t *testing.T) {
	if p, ok := npipePath("npipe:////./pipe/docker_engine"); !ok || p != `\\.\pipe\docker_engine` {
		t.Fatalf("got %q %v", p, ok)
	}
	if _, ok := npipePath("unix:///var/run/docker.sock"); ok {
		t.Fatal("unix host is not a pipe")
	}
}

type countingCollector struct {
	calls int
	err   error
}

func (c *countingCollector) Name() string { return "docker" }
func (c *countingCollector) Collect(_ context.Context, s *protocol.Sample) error {
	c.calls++
	if c.err != nil {
		return c.err
	}
	s.Docker = &protocol.Docker{}
	return nil
}

func TestLazyDockerAppearsAndDisappears(t *testing.T) {
	now := time.Unix(1000, 0)
	present, checks := false, 0
	inner := &countingCollector{}
	c := newLazyDocker(inner, func() bool { checks++; return present }, 60*time.Second, func() time.Time { return now })

	s := &protocol.Sample{}
	if err := c.Collect(context.Background(), s); err != nil || s.Docker != nil || s.Unavailable != nil || checks != 1 {
		t.Fatalf("absent: err %v docker %v unavailable %v checks %d", err, s.Docker, s.Unavailable, checks)
	}
	now = now.Add(30 * time.Second)
	c.Collect(context.Background(), &protocol.Sample{})
	if checks != 1 || inner.calls != 0 {
		t.Fatalf("rechecked too soon: checks %d calls %d", checks, inner.calls)
	}
	present = true
	now = now.Add(31 * time.Second)
	s = &protocol.Sample{}
	if err := c.Collect(context.Background(), s); err != nil || s.Docker == nil || inner.calls != 1 {
		t.Fatalf("present: err %v docker %v calls %d", err, s.Docker, inner.calls)
	}
	present, inner.err = false, errors.New("open \\\\.\\pipe\\docker_engine: file not found")
	s = &protocol.Sample{}
	if err := c.Collect(context.Background(), s); err != nil || s.Unavailable != nil || s.Docker != nil {
		t.Fatalf("disappeared: err %v unavailable %v", err, s.Unavailable)
	}
	now = now.Add(5 * time.Second)
	c.Collect(context.Background(), &protocol.Sample{})
	if inner.calls != 2 {
		t.Fatalf("called while known absent: %d", inner.calls)
	}
}

type slowDocker struct{ delay time.Duration }

func (slowDocker) Name() string { return "docker" }
func (d slowDocker) Collect(_ context.Context, s *protocol.Sample) error {
	time.Sleep(d.delay)
	s.Docker = &protocol.Docker{Containers: []protocol.Container{{Name: "web", State: "running"}}}
	return nil
}

func TestBackgroundDockerNeverBlocks(t *testing.T) {
	c := newBackgroundDocker(slowDocker{200 * time.Millisecond}, 15*time.Second)
	start := time.Now()
	s := &protocol.Sample{Time: time.Now()}
	if err := c.Collect(context.Background(), s); err != nil || s.Docker != nil {
		t.Fatalf("first sample: err %v docker %v", err, s.Docker)
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("blocked on the docker daemon")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s = &protocol.Sample{Time: time.Now()}
		c.Collect(context.Background(), s)
		if s.Docker != nil && s.Docker.Containers[0].Name == "web" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("docker data never arrived")
}

type goneDocker struct{}

func (goneDocker) Name() string                                    { return "docker" }
func (goneDocker) Collect(context.Context, *protocol.Sample) error { return nil }

func TestBackgroundDockerClearsWhenGone(t *testing.T) {
	c := newBackgroundDocker(goneDocker{}, time.Millisecond)
	for i := 0; i < 50; i++ {
		s := &protocol.Sample{Time: time.Now()}
		c.Collect(context.Background(), s)
		if s.Docker != nil {
			t.Fatal("no docker data must give no docker section")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
