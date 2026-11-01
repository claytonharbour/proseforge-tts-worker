//go:build unit

package inference

import "testing"

// TTS_ORT_INTRA_THREADS is opt-in (#113): anything that is not a positive integer must fall back
// to the ONNX Runtime default (0), so a typo can never change today's behaviour.
func TestIntraThreadsFromEnv(t *testing.T) {
	cases := []struct {
		set  bool
		val  string
		want int
	}{
		{false, "", 0},
		{true, "", 0},
		{true, "8", 8},
		{true, "1", 1},
		{true, "0", 0},
		{true, "-3", 0},
		{true, "eight", 0},
		{true, "8 ", 0},
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			if c.set {
				t.Setenv(intraThreadsEnv, c.val)
			} else {
				t.Setenv(intraThreadsEnv, "")
			}
			if got := intraThreadsFromEnv(); got != c.want {
				t.Errorf("TTS_ORT_INTRA_THREADS=%q -> %d, want %d", c.val, got, c.want)
			}
		})
	}
}
