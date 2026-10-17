package dockercli

import (
	"math"
	"testing"
)

func TestByteSize(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"19.56MiB / 31.04GiB", 19.56 * 1048576},
		{"4.75MB / 2GB", 4.75e6},
		{"1GiB / 2GiB", 1073741824},
		{"—", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := ByteSize(tt.input)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("ByteSize(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}
func TestTotalUsageDoesNotCountExitedContainers(t *testing.T) {
	s := Snapshot{
		Containers: []Container{{ID: "a", State: "running"}, {ID: "b", State: "exited"}, {ID: "c", Names: "db", State: "running"}},
		Stats:      map[string]Usage{"a": {CPUPerc: "8.5%", MemUsage: "10MiB / 2GiB"}, "b": {CPUPerc: "88%", MemUsage: "4GiB / 5GiB"}, "db": {CPUPerc: "2%", MemUsage: "20MiB / 2GiB"}},
	}
	cpu, memory := s.TotalUsage()
	if cpu != 10.5 || memory != 30*1024*1024 {
		t.Fatalf("cpu=%v memory=%v", cpu, memory)
	}
}
