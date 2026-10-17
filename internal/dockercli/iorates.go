package dockercli

import (
	"strings"
	"time"
)

// IOCounters holds per-container cumulative byte counters from docker stats.
// These are only comparable within one Docker context and one container ID.
type IOCounters struct{ RX, TX, Read, Write float64 }
type IORates struct {
	IOCounters
	Ready bool
}

func parseIOPair(input string) (float64, float64, bool) {
	pair := strings.Split(input, "/")
	if len(pair) != 2 {
		return 0, 0, false
	}
	left, right := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
	if left == "" || right == "" || left == "--" || right == "--" {
		return 0, 0, false
	}
	return ByteSize(left), ByteSize(right), true
}

func (s Snapshot) IOCounters() map[string]IOCounters {
	result := make(map[string]IOCounters)
	for _, c := range s.Containers {
		if c.State != "running" || c.ID == "" {
			continue
		}
		u, ok := s.Stats[c.ID]
		if !ok {
			u, ok = s.Stats[c.Names]
		}
		if !ok {
			continue
		}
		rx, tx, netOK := parseIOPair(u.NetIO)
		read, write, diskOK := parseIOPair(u.BlockIO)
		if !netOK && !diskOK {
			continue
		}
		result[c.ID] = IOCounters{RX: rx, TX: tx, Read: read, Write: write}
	}
	return result
}

// DeltaIO never interprets cumulative counters as rates, and discards
// a counter that decreased (reset/restart). New/missing containers contribute
// nothing until two successive valid samples exist.
func DeltaIO(old, now map[string]IOCounters, elapsed time.Duration) IORates {
	var rates IORates
	if elapsed < 500*time.Millisecond || elapsed > 60*time.Second {
		return rates
	}
	seconds := elapsed.Seconds()
	for id, next := range now {
		previous, ok := old[id]
		if !ok {
			continue
		}
		rates.Ready = true
		if next.RX >= previous.RX {
			rates.RX += (next.RX - previous.RX) / seconds
		}
		if next.TX >= previous.TX {
			rates.TX += (next.TX - previous.TX) / seconds
		}
		if next.Read >= previous.Read {
			rates.Read += (next.Read - previous.Read) / seconds
		}
		if next.Write >= previous.Write {
			rates.Write += (next.Write - previous.Write) / seconds
		}
	}
	return rates
}
