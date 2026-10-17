// Package hostmetrics reads local Linux host telemetry without using Docker,
// privileged APIs, background agents, or persistent subprocesses.
package hostmetrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Snapshot reports measurements from the local Linux host, never from the
// selected Docker context. CPUPercent needs two successive /proc/stat samples.
type Snapshot struct {
	CPUPercent              float64
	CPUReady                bool
	CPUCounters             Counters
	MemoryUsed, MemoryTotal uint64
	GPU                     GPU
	Updated                 time.Time
}

type Counters struct{ Total, Idle uint64 }
type GPU struct {
	Name                    string
	Utilization             float64
	MemoryUsed, MemoryTotal uint64
	Temperature             float64
	Available               bool
}

func parseCPU(input string) (Counters, error) {
	for _, line := range strings.Split(input, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var values []uint64
		for _, v := range fields[1:] {
			num, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return Counters{}, fmt.Errorf("invalid cpu counter: %w", err)
			}
			values = append(values, num)
		}
		var total uint64
		for _, value := range values {
			total += value
		}
		// iowait is idle time, guest/guest_nice are already accounted for in user/nice.
		if len(values) >= 10 {
			total -= values[8] + values[9]
		}
		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		return Counters{Total: total, Idle: idle}, nil
	}
	return Counters{}, errors.New("missing aggregate cpu counters")
}

func CPUPercent(old, next Counters) (float64, bool) {
	if old.Total == 0 || next.Total <= old.Total || next.Idle < old.Idle {
		return 0, false
	}
	total := next.Total - old.Total
	idle := next.Idle - old.Idle
	if idle > total {
		return 0, false
	}
	return 100 * float64(total-idle) / float64(total), true
}

func parseMemory(input string) (used, total uint64, err error) {
	var available uint64
	seen := map[string]bool{}
	for _, line := range strings.Split(input, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "kB" {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		value, e := strconv.ParseUint(fields[1], 10, 64)
		if e != nil {
			return 0, 0, e
		}
		seen[key] = true
		if key == "MemTotal" {
			total = value * 1024
		} else {
			available = value * 1024
		}
	}
	if !seen["MemTotal"] || !seen["MemAvailable"] || total == 0 || available > total {
		return 0, 0, errors.New("invalid MemTotal/MemAvailable")
	}
	return total - available, total, nil
}

// Read samples CPU and memory directly from procfs, with optional GPU discovery.
// Missing GPU sensors never make CPU and memory readings fail.
func Read(fetchGPU bool) (Snapshot, error) {
	var s Snapshot
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return s, err
	}
	s.CPUCounters, err = parseCPU(string(stat))
	if err != nil {
		return s, err
	}
	memory, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return s, err
	}
	s.MemoryUsed, s.MemoryTotal, err = parseMemory(string(memory))
	if err != nil {
		return s, err
	}
	s.Updated = time.Now()
	if fetchGPU {
		s.GPU = readGPU()
	}
	return s, nil
}

func parseNvidia(input string) (GPU, bool) {
	// nvidia-smi output: utilization %, used MiB, total MiB, temp C.
	line := strings.Split(strings.TrimSpace(input), "\n")[0]
	fields := strings.Split(line, ",")
	if len(fields) != 4 {
		return GPU{}, false
	}
	numbers := make([]float64, 4)
	for i, field := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil || v < 0 {
			return GPU{}, false
		}
		numbers[i] = v
	}
	if numbers[0] > 100 || numbers[1] > numbers[2] || numbers[2] <= 0 {
		return GPU{}, false
	}
	return GPU{Name: "NVIDIA", Utilization: numbers[0], MemoryUsed: uint64(numbers[1] * 1048576), MemoryTotal: uint64(numbers[2] * 1048576), Temperature: numbers[3], Available: true}, true
}

func readGPU() GPU {
	// Prefer a bounded, no-shell query. This is optional and runs only every
	// third 5-second sample; unsupported/unavailable drivers fail quietly.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=utilization.gpu,memory.used,memory.total,temperature.gpu", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err == nil && len(output) < 4096 {
		if gpu, ok := parseNvidia(string(output)); ok {
			return gpu
		}
	}
	// Some AMD (and selected Intel) Linux drivers expose a utilization file.
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]*/device/gpu_busy_percent")
	for _, path := range cards {
		data, e := os.ReadFile(path)
		if e != nil {
			continue
		}
		value, e := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if e != nil || value < 0 || value > 100 {
			continue
		}
		gpu := GPU{Name: "DRM", Utilization: value, Available: true}
		device := filepath.Dir(path)
		if used, e := os.ReadFile(filepath.Join(device, "mem_info_vram_used")); e == nil {
			if n, e := strconv.ParseUint(strings.TrimSpace(string(used)), 10, 64); e == nil {
				gpu.MemoryUsed = n
			}
		}
		if total, e := os.ReadFile(filepath.Join(device, "mem_info_vram_total")); e == nil {
			if n, e := strconv.ParseUint(strings.TrimSpace(string(total)), 10, 64); e == nil {
				gpu.MemoryTotal = n
			}
		}
		return gpu
	}
	return GPU{}
}
