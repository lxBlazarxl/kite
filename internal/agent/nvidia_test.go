package agent

import "testing"

const nvidiaCSV = "0, NVIDIA GeForce RTX 4060 Laptop GPU, 35, 1024, 8188, 52, 18.40\n1, Tesla T4, [N/A], 0, 16384, [N/A], [N/A]\n"

func TestParseNvidiaCSV(t *testing.T) {
	gpus, err := parseNvidiaCSV([]byte(nvidiaCSV))
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 2 {
		t.Fatalf("want 2, got %d", len(gpus))
	}
	g := gpus[0]
	if g.Name != "NVIDIA GeForce RTX 4060 Laptop GPU" || g.Vendor != "nvidia" || *g.UtilPct != 35 ||
		*g.VRAMUsedBytes != 1073741824 || *g.VRAMTotalBytes != 8585740288 || *g.TempC != 52 || *g.PowerW != 18.4 {
		t.Fatalf("gpu0 %+v", g)
	}
	g = gpus[1]
	if g.UtilPct != nil || g.TempC != nil || g.PowerW != nil || *g.VRAMUsedBytes != 0 {
		t.Fatalf("gpu1 %+v", g)
	}
	if _, err := parseNvidiaCSV([]byte("0, x, 1\n")); err == nil {
		t.Fatal("short line must be an error")
	}
}
