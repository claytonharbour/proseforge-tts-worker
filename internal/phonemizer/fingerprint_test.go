//go:build unit

package phonemizer

import "testing"

// The property #92 actually needs: a reading can change while the entry count
// stays put, and the fingerprint must still move. `bec2550` re-notated 8 homograph
// keys without adding or removing one, so a size-only check would have reported
// "no change" across a deploy that changed how words are pronounced.
func TestFingerprintMovesWhenAReadingChangesButTheCountDoesNot(t *testing.T) {
	before := fingerprintDict(&MisakiDict{entries: map[string]string{
		"karen": "kˈæɹən", "yonas": "jˈOnəs",
	}})
	after := fingerprintDict(&MisakiDict{entries: map[string]string{
		"karen": "kəɹˈɛn", "yonas": "jˈOnəs", // one reading re-notated
	}})

	if before.Entries != after.Entries {
		t.Fatalf("test is not exercising the intended case: counts differ (%d vs %d)",
			before.Entries, after.Entries)
	}
	if before.Digest == after.Digest {
		t.Errorf("digest did not move when a reading changed: %s — a size-only "+
			"check would have missed this, which is the defect #92 is about", before.Digest)
	}
}

// A change detector is only as good as its ability to go red, so pin the other
// direction too: identical content must produce an identical digest regardless of
// Go's randomised map iteration order.
func TestFingerprintIsStableAcrossMapIterationOrder(t *testing.T) {
	entries := map[string]string{}
	for _, w := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		entries[w] = "ˈ" + w
	}
	first := fingerprintDict(&MisakiDict{entries: entries})
	for i := 0; i < 24; i++ {
		if got := fingerprintDict(&MisakiDict{entries: entries}); got != first {
			t.Fatalf("digest is not deterministic: %+v then %+v", first, got)
		}
	}
}

// Two DIFFERENT dictionaries must not collide just because their concatenated
// bytes agree. Without a terminator after every field, {"ab": "c"} and {"a": "bc"}
// serialise identically.
func TestFingerprintDoesNotCollideOnFieldBoundaries(t *testing.T) {
	a := fingerprintDict(&MisakiDict{entries: map[string]string{"ab": "c"}})
	b := fingerprintDict(&MisakiDict{entries: map[string]string{"a": "bc"}})
	if a.Digest == b.Digest {
		t.Errorf("field boundary collision: {ab:c} and {a:bc} both hash to %s", a.Digest)
	}
}

// Red-proof for the context-list boundary. Writing ContextBefore and ContextAfter
// back to back makes before=[a] after=[b] indistinguishable from before=[a,b]
// after=[] — a swap that changes which reading a sentence resolves to while the
// digest sits still. This test failed before the "before"/"after" markers were
// added to fingerprintHomographs.
func TestFingerprintSeparatesContextBeforeFromContextAfter(t *testing.T) {
	split := fingerprintHomographs(&HomographResolver{entries: map[string]HomographEntry{
		"read": {Default: "ɹˈɛd", Pronunciations: map[string]HomographContext{
			"ɹˈid": {ContextBefore: []string{"will"}, ContextAfter: []string{"aloud"}},
		}},
	}})
	merged := fingerprintHomographs(&HomographResolver{entries: map[string]HomographEntry{
		"read": {Default: "ɹˈɛd", Pronunciations: map[string]HomographContext{
			"ɹˈid": {ContextBefore: []string{"will", "aloud"}, ContextAfter: nil},
		}},
	}})
	if split.Digest == merged.Digest {
		t.Errorf("context lists are not delimited: before=[will] after=[aloud] hashes "+
			"identically to before=[will,aloud] after=[] (%s)", split.Digest)
	}
}

// The silent bank-drop, made visible. On the binary dev is running, a malformed
// custom_dict.json yields a nil bank and a working phonemizer with nothing logged;
// the fingerprint reports zero entries, which is the signal that was missing.
func TestDroppedCustomBankIsVisibleAsZeroEntries(t *testing.T) {
	loaded := fingerprintDict(&MisakiDict{entries: map[string]string{"lina": "lˈinə"}})
	dropped := fingerprintDict(nil)

	if dropped.Entries != 0 {
		t.Errorf("a nil bank should report 0 entries, got %d", dropped.Entries)
	}
	if loaded.Digest == dropped.Digest {
		t.Error("a dropped bank is indistinguishable from a loaded one")
	}
	if dropped.Digest == "" {
		t.Error("a nil bank must still carry a digest, so the field is never absent")
	}
}

// The vocab digest must cover the ids, not only the symbols: a remapped id changes
// which sound the model receives while leaving the symbol set identical.
func TestVocabFingerprintCoversTokenIDsNotJustSymbols(t *testing.T) {
	a := fingerprintVocab(&Tokenizer{vocab: map[rune]int64{'a': 1, 'b': 2}})
	b := fingerprintVocab(&Tokenizer{vocab: map[rune]int64{'a': 2, 'b': 1}})
	if a.Entries != b.Entries {
		t.Fatalf("test not exercising the intended case: %d vs %d", a.Entries, b.Entries)
	}
	if a.Digest == b.Digest {
		t.Errorf("swapping two token ids left the digest unchanged (%s)", a.Digest)
	}
}

// End to end over the real shipped data: every bank must be non-empty and every
// digest distinct, so a wiring mistake that fingerprinted the same map five times
// cannot pass. Honours MISAKI_DATA_DIR via newTestPhonemizer.
func TestDataFingerprintCoversEveryBankOverShippedData(t *testing.T) {
	fp := newTestPhonemizer(t).DataFingerprint()

	banks := map[string]BankFingerprint{
		"gold": fp.Gold, "silver": fp.Silver, "custom": fp.Custom,
		"homographs": fp.Homographs, "vocab": fp.Vocab,
	}
	seen := map[string]string{}
	for name, b := range banks {
		if b.Entries == 0 {
			t.Errorf("%s fingerprinted as empty over shipped data", name)
		}
		if prev, dup := seen[b.Digest]; dup {
			t.Errorf("%s and %s share digest %s — a bank is being hashed twice",
				prev, name, b.Digest)
		}
		seen[b.Digest] = name
		t.Logf("  %-11s %6d entries  %s", name, b.Entries, b.Digest)
	}
}
