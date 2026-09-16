package sbx

import "testing"

func TestParseStats(t *testing.T) {
	output := `cpus 8
mem_total 16384000
mem_available 4096000
cpu  100 20 30 500 40 5 6 7 0 0
`
	sample, err := ParseStats(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sample.CPUs != 8 || sample.MemoryTotalKB != 16384000 || sample.MemoryAvailKB != 4096000 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	// 100+20+30+500+40+5+6+7 = 708 total, idle+iowait = 540, busy = 168.
	if sample.CPUTotal != 708 || sample.CPUBusy != 168 {
		t.Fatalf("unexpected cpu counters: %+v", sample)
	}
}

func TestParseStatsToleratesGaps(t *testing.T) {
	// awk can be unavailable for meminfo while /proc/stat still answers.
	sample, err := ParseStats("cpus 2\ncpu 10 0 0 90 0 0 0 0\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sample.MemoryTotalKB != 0 || sample.CPUs != 2 || sample.CPUTotal != 100 || sample.CPUBusy != 10 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
}

func TestParseStatsEmpty(t *testing.T) {
	if _, err := ParseStats("\n"); err == nil {
		t.Fatal("expected an error for empty output")
	}
}
