package dockercli

import (
	"math"
	"testing"
	"time"
)

func TestSnapshotIOAndDelta(t *testing.T) {
	old := Snapshot{Containers: []Container{{ID: "abc123", Names: "name", State: "running"}}, Stats: map[string]Usage{"abc123": {NetIO: "1MB / 500kB", BlockIO: "2MB / 3MB"}}}
	next := Snapshot{Containers: old.Containers, Stats: map[string]Usage{"abc123": {NetIO: "2MB / 700kB", BlockIO: "2.2MB / 3.5MB"}}}
	oldCounters, newCounters := old.IOCounters(), next.IOCounters()
	if len(oldCounters) != 1 || len(newCounters) != 1 {
		t.Fatal("missing counters")
	}
	r := DeltaIO(oldCounters, newCounters, 2*time.Second)
	if !r.Ready || math.Abs(r.RX-500000) > 0.01 || math.Abs(r.TX-100000) > 0.01 || math.Abs(r.Read-100000) > 0.01 || math.Abs(r.Write-250000) > 0.01 {
		t.Fatalf("rates: %+v", r)
	}
	if DeltaIO(oldCounters, newCounters, 0).Ready {
		t.Fatal("zero elapsed accepted")
	}
	if DeltaIO(map[string]IOCounters{}, newCounters, time.Second).Ready {
		t.Fatal("new container counted")
	}
	reset := DeltaIO(newCounters, oldCounters, time.Second)
	if !reset.Ready || reset.RX != 0 || reset.TX != 0 {
		t.Fatalf("restart produced negative rate %+v", reset)
	}
}
func TestCountersDoNotDuplicateNameAndID(t *testing.T) {
	s := Snapshot{Containers: []Container{{ID: "id", Names: "name", State: "running"}}, Stats: map[string]Usage{"name": {NetIO: "4kB / 1kB"}, "id": {NetIO: "4kB / 1kB"}}}
	if len(s.IOCounters()) != 1 {
		t.Fatal("duplicate counters")
	}
	s.Containers[0].State = "exited"
	if len(s.IOCounters()) != 0 {
		t.Fatal("exited container counted")
	}
}
