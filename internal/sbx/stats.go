package sbx

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
)

// StatsSample is one resource reading of a running sandbox, taken inside the
// sandbox with a short shell command (nproc, /proc/meminfo, /proc/stat).
// CPUBusy/CPUTotal are jiffy counters: usage needs two samples to be derived.
type StatsSample struct {
	CPUs          int    `json:"cpus"`
	MemoryTotalKB int64  `json:"memory_total_kb"`
	MemoryAvailKB int64  `json:"memory_available_kb"`
	CPUBusy       uint64 `json:"cpu_busy"`
	CPUTotal      uint64 `json:"cpu_total"`
}

// statsScript is the probe run inside the sandbox. It mirrors what the sbx TUI
// samples and only relies on sh, nproc and awk.
const statsScript = `printf 'cpus %s\n' "$(nproc 2>/dev/null || echo 0)"
awk '/^MemTotal/{print "mem_total", $2} /^MemAvailable/{print "mem_available", $2}' /proc/meminfo 2>/dev/null
awk '/^cpu /{print "cpu", $2, $3, $4, $5, $6, $7, $8, $9}' /proc/stat 2>/dev/null`

// Stats samples CPU and memory usage of a running sandbox. The sandbox must be
// running; the exec runs inside it, so it costs one child process per call and
// counts against MaxConcurrentCLI.
func (c *Client) Stats(ctx context.Context, sandbox string) (StatsSample, error) {
	release, err := c.acquireCLI(ctx)
	if err != nil {
		return StatsSample{}, err
	}
	defer release()
	var buf bytes.Buffer
	if err := c.Exec(ctx, sandbox, []string{"sh", "-c", statsScript}, &buf); err != nil {
		return StatsSample{}, err
	}
	return ParseStats(buf.String())
}

// ParseStats decodes the keyed lines produced by statsScript.
func ParseStats(output string) (StatsSample, error) {
	var out StatsSample
	var sawCPU bool
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "cpus":
			out.CPUs, _ = strconv.Atoi(fields[1])
		case "mem_total":
			out.MemoryTotalKB, _ = strconv.ParseInt(fields[1], 10, 64)
		case "mem_available":
			out.MemoryAvailKB, _ = strconv.ParseInt(fields[1], 10, 64)
		case "cpu":
			if len(fields) < 6 {
				continue
			}
			var total uint64
			var counters []uint64
			for _, field := range fields[1:] {
				value, err := strconv.ParseUint(field, 10, 64)
				if err != nil {
					counters = nil
					break
				}
				counters = append(counters, value)
				total += value
			}
			if len(counters) < 5 {
				continue
			}
			// user nice system idle iowait irq softirq steal
			out.CPUTotal = total
			out.CPUBusy = total - counters[3] - counters[4]
			sawCPU = true
		}
	}
	if out.MemoryTotalKB == 0 && !sawCPU {
		return StatsSample{}, errors.New("no stats output from the sandbox")
	}
	return out, nil
}
