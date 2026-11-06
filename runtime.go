package main

import (
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// tuneRuntime adjusts the garbage collector for this workload: a tiny live
// heap and a very high rate of short-lived allocations from net/http.
//
//   - GOGC defaults to 200 rather than 100, trading a little memory for half
//     as many GC cycles. Set GOGC to override.
//   - When running under a cgroup memory limit (containers, systemd
//     MemoryMax=) and GOMEMLIMIT is not set, the soft memory limit is set to
//     90% of it so the higher GOGC cannot push the process into the OOM
//     killer. Set GOMEMLIMIT to override.
func tuneRuntime() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(200)
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		if limit, ok := cgroupMemoryLimit(); ok {
			debug.SetMemoryLimit(limit / 10 * 9)
		}
	}
}

var cgroupLimitFiles = []string{
	"/sys/fs/cgroup/memory.max",                   // cgroup v2
	"/sys/fs/cgroup/memory/memory.limit_in_bytes", // cgroup v1
}

func cgroupMemoryLimit() (int64, bool) {
	for _, path := range cgroupLimitFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return parseCgroupLimit(string(data))
	}
	return 0, false
}

// parseCgroupLimit parses a cgroup memory limit file. "max" (v2) and very
// large values (v1 uses a number close to MaxInt64) mean no limit.
func parseCgroupLimit(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n >= math.MaxInt64/2 {
		return 0, false
	}
	return n, true
}
