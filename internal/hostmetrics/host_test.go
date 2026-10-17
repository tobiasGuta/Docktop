package hostmetrics

import (
	"math"
	"strings"
	"testing"
)

func TestCPUAccounting(t *testing.T) {
	old, e := parseCPU("cpu 100 0 100 700 100 0 0 0 10 10\ncpu0 1 1 1 1\n")
	if e != nil || old.Total != 1000 || old.Idle != 800 {
		t.Fatalf("old %v %v", old, e)
	}
	next, e := parseCPU("cpu 130 0 140 750 110 0 0 0 10 10\n")
	if e != nil {
		t.Fatal(e)
	}
	cpu, ok := CPUPercent(old, next)
	if !ok || math.Abs(cpu-(70.0/130.0*100)) > 0.01 {
		t.Fatalf("cpu %v ready %v", cpu, ok)
	}
	if _, ok := CPUPercent(next, old); ok {
		t.Fatal("accepted stale counters")
	}
	if _, err := parseCPU("cpu nope 4 4 4\n"); err == nil {
		t.Fatal("bad CPU input")
	}
}
func TestMemInfo(t *testing.T) {
	used, total, err := parseMemory("MemTotal: 32768 kB\nMemFree: 100 kB\nMemAvailable: 12288 kB\n")
	if err != nil || used != 20480*1024 || total != 32768*1024 {
		t.Fatalf("%d %d %v", used, total, err)
	}
	if _, _, e := parseMemory("MemTotal: 10 kB\n"); e == nil {
		t.Fatal("missing available")
	}
	if _, _, e := parseMemory("MemTotal: 10 kB\nMemAvailable: 11 kB\n"); e == nil {
		t.Fatal("invalid available")
	}
}
func TestGPUParsing(t *testing.T) {
	gpu, ok := parseNvidia("5, 316, 4096, 42\n3, 20, 1024, 30")
	if !ok || gpu.MemoryTotal != 4096*1048576 || gpu.Utilization != 5 {
		t.Fatalf("%+v %v", gpu, ok)
	}
	for _, v := range []string{"N/A, 2, 5, 6", "3,5,2,6", "150,4,8,40", strings.Repeat("1,", 6)} {
		if _, ok := parseNvidia(v); ok {
			t.Fatalf("accepted invalid %q", v)
		}
	}
}
