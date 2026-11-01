package phonemizer

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

// BankFingerprint identifies one loaded lookup table.
//
// Entries is the count the resolver can actually serve, and Digest covers the
// keys AND their readings — so a dictionary that keeps its size while a reading
// changes still moves. Size alone would not: `bec2550` re-notated 8 homograph
// keys without adding or removing one.
type BankFingerprint struct {
	Entries int    `json:"entries"`
	Digest  string `json:"digest"`
}

// DataFingerprint identifies every table the phonemizer resolved at load time.
//
// ⚠️ This describes what is IN MEMORY, never what is on disk, and the distinction
// is the entire reason it exists (#92). A data-only deploy replaces JSON files and
// restarts the worker; the build hash cannot move, because nothing about the build
// changed. Hashing the files instead would report the bytes present now rather than
// the bytes this process parsed — which are different values whenever a deploy lands
// while a worker is running, exactly the case worth detecting.
//
// ⛔ It is a CHANGE DETECTOR, not a validator. It answers "is this worker serving the
// dictionary I think it is", never "is that dictionary correct" — LoadMisakiDict and
// ValidateAgainstVocab answer the second.
//
// 📌 Deliberately NOT the same thing as dataFingerprint in cmd/diagnose, which hashes
// the *.json FILES in a directory. That one identifies a data dir an operator points a
// CLI at; this one identifies the maps a live process is holding. They disagree exactly
// when it matters — after a data drop that no worker has reloaded — so neither replaces
// the other and the file-level one is left alone.
type DataFingerprint struct {
	Gold       BankFingerprint `json:"gold"`
	Silver     BankFingerprint `json:"silver"`
	Custom     BankFingerprint `json:"custom"`
	Homographs BankFingerprint `json:"homographs"`
	Vocab      BankFingerprint `json:"vocab"`
}

// digestOf renders a fingerprint from an already-sorted walk.
//
// Keys are sorted and every field is written with an explicit terminator, so
// {"ab": "c"} and {"a": "bc"} cannot collide into the same digest.
func digestOf(n int, walk func(write func(parts ...string))) BankFingerprint {
	h := sha256.New()
	walk(func(parts ...string) {
		for _, p := range parts {
			_, _ = h.Write([]byte(p))
			_, _ = h.Write([]byte{0})
		}
	})
	return BankFingerprint{Entries: n, Digest: hex.EncodeToString(h.Sum(nil))[:16]}
}

// fingerprintDict hashes a loaded Misaki dictionary. A nil dict is a legitimate
// state for the optional custom bank and fingerprints as zero entries — which is
// itself the signal worth seeing, since a silently dropped bank is otherwise
// indistinguishable from a worse voice.
func fingerprintDict(d *MisakiDict) BankFingerprint {
	if d == nil {
		return digestOf(0, func(write func(...string)) {})
	}
	words := make([]string, 0, len(d.entries))
	for w := range d.entries {
		words = append(words, w)
	}
	sort.Strings(words)
	return digestOf(len(words), func(write func(...string)) {
		for _, w := range words {
			write(w, d.entries[w])
		}
	})
}

// fingerprintHomographs hashes the loaded homograph rules, including every
// context reading rather than only the default — the context branches are where
// #63's re-notation landed, and a default-only digest would have missed it.
func fingerprintHomographs(r *HomographResolver) BankFingerprint {
	if r == nil {
		return digestOf(0, func(write func(...string)) {})
	}
	words := make([]string, 0, len(r.entries))
	for w := range r.entries {
		words = append(words, w)
	}
	sort.Strings(words)
	return digestOf(len(words), func(write func(...string)) {
		for _, w := range words {
			e := r.entries[w]
			write(w, e.Default, e.Imperative, e.InflectedDefault)
			// Pronunciations is keyed BY THE READING, with the context triggers
			// as the value — so the map keys are themselves phoneme strings and
			// belong in the digest, and so do the triggers that select them: a
			// changed trigger changes which reading a sentence resolves to.
			prons := make([]string, 0, len(e.Pronunciations))
			for pron := range e.Pronunciations {
				prons = append(prons, pron)
			}
			sort.Strings(prons)
			for _, pron := range prons {
				ctx := e.Pronunciations[pron]
				// ⚠️ The two trigger lists need an explicit boundary. Writing them
				// back to back makes before=[a] after=[b] serialise identically to
				// before=[a,b] after=[] — a swap that changes resolution and would
				// leave the digest still. A change detector blind to a change is
				// the exact failure this endpoint exists to end.
				write(pron)
				write("before")
				write(ctx.ContextBefore...)
				write("after")
				write(ctx.ContextAfter...)
			}
		}
	})
}

// fingerprintVocab hashes the token vocabulary. The ids are included, not just the
// runes: a remapped id changes which sound the model receives while leaving the
// symbol set identical.
func fingerprintVocab(t *Tokenizer) BankFingerprint {
	if t == nil {
		return digestOf(0, func(write func(...string)) {})
	}
	runes := make([]rune, 0, len(t.vocab))
	for r := range t.vocab {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	return digestOf(len(runes), func(write func(...string)) {
		var buf [8]byte
		for _, r := range runes {
			binary.BigEndian.PutUint64(buf[:], uint64(t.vocab[r]))
			write(string(r), string(buf[:]))
		}
	})
}

// DataFingerprint reports what this phonemizer actually loaded.
func (p *Phonemizer) DataFingerprint() DataFingerprint {
	return DataFingerprint{
		Gold:       fingerprintDict(p.gold),
		Silver:     fingerprintDict(p.silver),
		Custom:     fingerprintDict(p.custom),
		Homographs: fingerprintHomographs(p.homographs),
		Vocab:      fingerprintVocab(p.tokenizer),
	}
}
