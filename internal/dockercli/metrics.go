package dockercli

import (
	"strconv"
	"strings"
)

// ByteSize converts a Docker stats memory quantity into bytes. A zero value
// also covers unavailable or malformed readings; it is never estimated.
func ByteSize(input string) float64 {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return 0
	}
	token := fields[0]
	n := 0
	for n < len(token) && (token[n] == '.' || token[n] >= '0' && token[n] <= '9') {
		n++
	}
	value, err := strconv.ParseFloat(token[:n], 64)
	if err != nil || value < 0 {
		return 0
	}
	units := strings.ToLower(token[n:])
	switch units {
	case "b", "":
		return value
	case "kb":
		return value * 1e3
	case "mb":
		return value * 1e6
	case "gb":
		return value * 1e9
	case "tb":
		return value * 1e12
	case "kib":
		return value * 1024
	case "mib":
		return value * 1024 * 1024
	case "gib":
		return value * 1024 * 1024 * 1024
	case "tib":
		return value * 1024 * 1024 * 1024 * 1024
	default:
		return 0
	}
}

// Percent converts Docker's CPU and memory percentage strings to numbers.
func Percent(input string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(input), "%"), 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func (s Snapshot) TotalUsage() (cpu, bytes float64) {
	for _, container := range s.Containers {
		if container.State != "running" {
			continue
		}
		usage, ok := s.Stats[container.ID]
		if !ok {
			usage, ok = s.Stats[container.Names]
		}
		if !ok {
			continue
		}
		cpu += Percent(usage.CPUPerc)
		bytes += ByteSize(usage.MemUsage)
	}
	return cpu, bytes
}
