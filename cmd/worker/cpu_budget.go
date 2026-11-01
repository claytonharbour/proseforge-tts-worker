package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// cpuBudget describes the CPUs this process is allowed to use, for one startup log line (#115).
//
// Why it exists: the best TTS_ORT_INTRA_THREADS value is the number of cores a worker really has,
// and on RunPod nobody knew that number. runtime.NumCPU() is not it: in a container it counts the
// CPUs the host lets the process see, not the cgroup limit. So this reads the limit itself and
// prints every figure next to the others, because they disagree exactly when it matters.
type cpuBudget struct {
	NumCPU     int
	GoMaxProcs int
	// Quota is the cgroup CPU limit in cores ("max" = no limit, "n/a" = no cgroup file found).
	Quota string
	// Allowed is the cgroup cpuset list ("0-3,8"), or "n/a".
	Allowed string
	// IntraThreads is the ONNX thread count in effect: 0 means the ONNX Runtime default.
	IntraThreads int
}

// readCPUBudget collects the figures. readFile is os.ReadFile in production and a map in tests.
func readCPUBudget(readFile func(string) ([]byte, error), numCPU, gomaxprocs, intraThreads int) cpuBudget {
	read := func(path string) (string, bool) {
		b, err := readFile(path)
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(b)), true
	}
	b := cpuBudget{NumCPU: numCPU, GoMaxProcs: gomaxprocs, IntraThreads: intraThreads, Quota: "n/a", Allowed: "n/a"}

	// cgroup v2: "<quota> <period>" or "max <period>".
	if v, ok := read("/sys/fs/cgroup/cpu.max"); ok {
		b.Quota = quotaCores(strings.Fields(v))
	} else if q, ok := read("/sys/fs/cgroup/cpu/cpu.cfs_quota_us"); ok {
		// cgroup v1: quota and period are separate files; a quota of -1 means no limit.
		if p, ok := read("/sys/fs/cgroup/cpu/cpu.cfs_period_us"); ok {
			if q == "-1" {
				q = "max"
			}
			b.Quota = quotaCores([]string{q, p})
		}
	}

	if v, ok := read("/sys/fs/cgroup/cpuset.cpus.effective"); ok && v != "" {
		b.Allowed = v
	} else if v, ok := read("/sys/fs/cgroup/cpuset/cpuset.effective_cpus"); ok && v != "" {
		b.Allowed = v
	}
	return b
}

// quotaCores turns {quota, period} into "2" or "1.5" cores, "max" for no limit, "n/a" if unreadable.
func quotaCores(f []string) string {
	if len(f) != 2 {
		return "n/a"
	}
	if f[0] == "max" {
		return "max"
	}
	quota, err1 := strconv.ParseFloat(f[0], 64)
	period, err2 := strconv.ParseFloat(f[1], 64)
	if err1 != nil || err2 != nil || period <= 0 || quota <= 0 {
		return "n/a"
	}
	return strconv.FormatFloat(quota/period, 'f', -1, 64)
}

func (b cpuBudget) String() string {
	threads := "default"
	if b.IntraThreads > 0 {
		threads = strconv.Itoa(b.IntraThreads)
	}
	return fmt.Sprintf("cpu budget: NumCPU=%d GOMAXPROCS=%d cgroup_quota_cores=%s cgroup_cpuset=%s ort_intra_threads=%s",
		b.NumCPU, b.GoMaxProcs, b.Quota, b.Allowed, threads)
}

// logCPUBudget prints the line once at startup, so a worker's log says what it was given.
func logCPUBudget(intraThreads int) {
	log.Print(readCPUBudget(os.ReadFile, runtime.NumCPU(), runtime.GOMAXPROCS(0), intraThreads))
}
