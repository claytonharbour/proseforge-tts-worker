//go:build unit

package main

import (
	"errors"
	"testing"
)

// The startup line must report the CPU limit the worker was GIVEN, not the host's core count (#115).
// Each case feeds the cgroup files a container would show and states what a reader should see; the
// "host shows 64, limit is 2" cases are the reason the line exists: NumCPU alone would say 64.
func TestCPUBudgetLine(t *testing.T) {
	files := func(m map[string]string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if v, ok := m[p]; ok {
				return []byte(v + "\n"), nil
			}
			return nil, errors.New("not found")
		}
	}
	cases := []struct {
		name    string
		files   map[string]string
		threads int
		want    string
	}{
		{"cgroup v2, limited to 2 cores on a 64-core host",
			map[string]string{"/sys/fs/cgroup/cpu.max": "200000 100000", "/sys/fs/cgroup/cpuset.cpus.effective": "0-63"},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=2 cgroup_cpuset=0-63 ort_intra_threads=default"},
		{"cgroup v2, fractional quota",
			map[string]string{"/sys/fs/cgroup/cpu.max": "150000 100000"},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=1.5 cgroup_cpuset=n/a ort_intra_threads=default"},
		{"cgroup v2, no limit",
			map[string]string{"/sys/fs/cgroup/cpu.max": "max 100000"},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=max cgroup_cpuset=n/a ort_intra_threads=default"},
		{"cgroup v1, limited",
			map[string]string{"/sys/fs/cgroup/cpu/cpu.cfs_quota_us": "400000", "/sys/fs/cgroup/cpu/cpu.cfs_period_us": "100000",
				"/sys/fs/cgroup/cpuset/cpuset.effective_cpus": "0-3,8"},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=4 cgroup_cpuset=0-3,8 ort_intra_threads=default"},
		{"cgroup v1, quota -1 is no limit",
			map[string]string{"/sys/fs/cgroup/cpu/cpu.cfs_quota_us": "-1", "/sys/fs/cgroup/cpu/cpu.cfs_period_us": "100000"},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=max cgroup_cpuset=n/a ort_intra_threads=default"},
		{"no cgroup files at all (a laptop) says so, and does not guess",
			map[string]string{},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=n/a cgroup_cpuset=n/a ort_intra_threads=default"},
		{"unreadable quota is n/a, not zero",
			map[string]string{"/sys/fs/cgroup/cpu.max": "garbage"},
			0, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=n/a cgroup_cpuset=n/a ort_intra_threads=default"},
		{"a thread override in effect is shown as a number",
			map[string]string{},
			6, "cpu budget: NumCPU=64 GOMAXPROCS=2 cgroup_quota_cores=n/a cgroup_cpuset=n/a ort_intra_threads=6"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := readCPUBudget(files(c.files), 64, 2, c.threads).String(); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
