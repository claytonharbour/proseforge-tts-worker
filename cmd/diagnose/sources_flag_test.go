//go:build unit

package main

import (
	"flag"
	"io"
	"reflect"
	"strings"
	"testing"
)

// #47's observability gap, in the part of it this flag closes: only lts/silver/skip
// inventories were emitted, so a gold- or custom-resolved word's phonemes could not be
// read back. That blocks any check comparing a dictionary entry against what the engine
// actually produces for it — which is exactly what @wayland's bank-drift pin needs, and
// the reason his version can only watch the LTS-resolved subset.
//
// The three default sources are dropped from the list rather than accepted, because
// asking for one would print its section twice: the risk-surface loop already emits it.
func TestParseSourcesDropsTheDefaultsAndSorts(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"gold", []string{"gold"}},
		{"custom,gold", []string{"custom", "gold"}},
		{"gold,custom", []string{"custom", "gold"}}, // sorted, so output order is stable
		{" gold , custom ", []string{"custom", "gold"}},
		{"lts", nil},                                 // already emitted
		{"lts,silver,skip", nil},                     // all three already emitted
		{"lts,gold", []string{"gold"}},               // the default is dropped, the extra kept
		{"gold,,custom", []string{"custom", "gold"}}, // empty element from a trailing comma
	} {
		if got := parseSources(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseSources(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// A typo in -sources used to produce no inventory, no warning and exit 0 — the same
// output as omitting the flag, so "there are no gold words" and "you misspelled gold"
// were indistinguishable. @aldric's silently-ignored-parameter trap: he watched a
// backend return total=125 unused=125 because it ignored an unrecognised query
// parameter, which looks exactly like a filter that matched everything.
//
// lts/silver/skip are accepted-but-ignored rather than rejected: they name real
// sources that are always emitted, so asking for one is redundant, not wrong.
func TestUnknownSourcesAreRejected(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"gold", nil},
		{"custom,gold,homograph,builtin", nil},
		{"lts,silver,skip", nil}, // redundant, not unknown
		{"golld", []string{"golld"}},
		{"banana", []string{"banana"}},
		{"gold,banana", []string{"banana"}},
		{"banana,apple", []string{"apple", "banana"}}, // sorted, so the message is stable
		{" gold , golld ", []string{"golld"}},
	} {
		if got := unknownSources(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("unknownSources(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// The two lists must not drift apart: every name parseSources will act on has to be
// one unknownSources accepts, or a valid flag value fails validation and never runs.
func TestEveryInventoriableSourceIsAccepted(t *testing.T) {
	for _, s := range inventoriableSources {
		if bad := unknownSources(s); len(bad) > 0 {
			t.Errorf("%q is in inventoriableSources but unknownSources rejects it — "+
				"the flag would refuse a value it then knows how to handle", s)
		}
		if got := parseSources(s); len(got) != 1 || got[0] != s {
			t.Errorf("parseSources(%q) = %v — accepted by validation but dropped by "+
				"the parser, so it would silently do nothing", s, got)
		}
	}
}

// Text-mode-only flags used to be accepted and silently ignored in audio mode:
// `-audio x.wav -sources gold -data-dir /nonexistent` ran a clean analysis with both
// flags never read. @wayland's class 2, found by auditing his own tool after I fixed
// the -sources typo case.
//
// The list is asserted rather than the behaviour because flag.Visit needs a parsed
// command line; what can rot here is the SET, if a new -text-only flag is added and
// not registered. A flag missing from this list is silently ignored in audio mode,
// which is the original defect.
func TestTextOnlyFlagListCoversTheTextModeFlags(t *testing.T) {
	// Every flag runTextDiagnosis consumes, by name as registered.
	consumedByTextMode := []string{"structural", "sources", "data-dir"}

	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Bool("structural", false, "")
	fs.String("sources", "", "")
	fs.String("data-dir", "data", "")
	if err := fs.Parse([]string{"-structural=false"}); err != nil {
		t.Fatal(err)
	}

	// flag.Visit reports a flag given its own default value — the property the check
	// rests on, since *structural == false cannot distinguish that from absent.
	var visited []string
	fs.Visit(func(f *flag.Flag) { visited = append(visited, f.Name) })
	if len(visited) != 1 || visited[0] != "structural" {
		t.Fatalf("Visit reported %v, want [structural] — a flag set to its default must "+
			"still count as set, or the audio-mode guard cannot see it", visited)
	}

	for _, name := range consumedByTextMode {
		if !strings.Contains(strings.Join(textOnlyFlagNames(), ","), name) {
			t.Errorf("%q is consumed by -text mode but is not in the text-only list — "+
				"passing it with -audio would be accepted and ignored", name)
		}
	}
}
