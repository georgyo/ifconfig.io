package main

import "testing"

func TestParseCgroupLimit(t *testing.T) {
	for in, want := range map[string]int64{
		"536870912\n":         536870912,
		"max\n":               0,
		"9223372036854771712": 0, // cgroup v1 "unlimited"
		"":                    0,
		"garbage":             0,
		"-1":                  0,
	} {
		got, ok := parseCgroupLimit(in)
		if got != want || ok != (want != 0) {
			t.Errorf("parseCgroupLimit(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
}
