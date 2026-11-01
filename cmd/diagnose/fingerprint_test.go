//go:build unit

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// dataFingerprint is what makes two measurements comparable across benches, and
// until 2026-08-19 nobody had made it go red. @wayland's point on #71 is the
// reason it exists: "a corpus date is not enough provenance when the
// dictionaries move hourly" — data/ took five commits in one evening, and he
// published an instrument caveat that was really a stale-vs-fresh comparison.
//
// 🛑 A FINGERPRINT THAT NEVER CHANGES IS WORSE THAN NONE, because every report
// carrying it reads as provenance. So this asserts all three directions, not
// just the interesting one:
//
//	stable across identical runs   ← the positive control. A function returning a
//	                                 constant passes the "differs" arm trivially
//	                                 if you only test that data changed it.
//	differs on a one-entry edit    ← the property actually being claimed
//	returns to baseline on restore ← rules out a time/counter component, which
//	                                 would make it differ for the wrong reason
//
// ⚑ Measured live the same day: adding one custom_dict entry moved
// `6 tables, 4f826b4fb963` → `6 tables, 8001e338d075`, and restoring returned it
// exactly. That baseline is byte-identical to the fingerprint @wayland quoted on
// #25, which is the whole point — it proved his corpus counts and my engine runs
// were on the same data without either of us describing what we had.
func TestDataFingerprintDiscriminates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.json", `{"x":1}`)
	write("b.json", `{"y":2}`)

	base, err := dataFingerprint(dir)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	if again, err := dataFingerprint(dir); err != nil || again != base {
		t.Fatalf("NOT STABLE across identical runs: %q vs %q (err %v) — a fingerprint "+
			"that drifts on its own cannot certify that two reports read the same data", base, again, err)
	}

	write("a.json", `{"x":2}`)
	changed, err := dataFingerprint(dir)
	if err != nil {
		t.Fatalf("after edit: %v", err)
	}
	if changed == base {
		t.Fatalf("DID NOT CHANGE after editing a data file (%q) — every report carrying "+
			"this fingerprint has been claiming provenance it does not have", base)
	}

	write("a.json", `{"x":1}`)
	if restored, err := dataFingerprint(dir); err != nil || restored != base {
		t.Fatalf("did not return to baseline after restore: %q vs %q (err %v) — the value "+
			"depends on something other than file content, so it cannot be compared across runs",
			base, restored, err)
	}

	// The empty-dir guard the doc comment credits to @wayland's regress.sh:
	// a hash of nothing is stable and normal-looking, which is the worst failure.
	if _, err := dataFingerprint(t.TempDir()); err == nil {
		t.Error("an empty data dir produced a fingerprint instead of an error — " +
			"a hash of nothing looks exactly like a hash of something")
	}
}
