//go:build unit

package audio

import (
	"encoding/json"
	"testing"
)

// makeToken builds a whisperTokenRaw for testing.
func makeToken(text string, from, to int, p float64) whisperTokenRaw {
	var tok whisperTokenRaw
	tok.Text = text
	tok.Offsets.From = from
	tok.Offsets.To = to
	tok.P = p
	return tok
}

func TestParseWhisperJSON_BasicWordReconstruction(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken("[_BEG_]", 0, 0, 1.0),
					makeToken(" Hello", 0, 500, 0.95),
					makeToken(" world", 500, 1000, 0.90),
				},
			},
		},
	}

	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	if len(alignment.Words) != 2 {
		t.Fatalf("got %d words, want 2", len(alignment.Words))
	}

	if alignment.Words[0].Word != "Hello" {
		t.Errorf("word[0] = %q, want %q", alignment.Words[0].Word, "Hello")
	}
	if alignment.Words[1].Word != "world" {
		t.Errorf("word[1] = %q, want %q", alignment.Words[1].Word, "world")
	}

	if alignment.Words[0].StartMs != 0 || alignment.Words[0].EndMs != 500 {
		t.Errorf("word[0] timing = [%d, %d], want [0, 500]",
			alignment.Words[0].StartMs, alignment.Words[0].EndMs)
	}
}

func TestParseWhisperJSON_MultiTokenWord(t *testing.T) {
	// "understanding" might be split into " under" + "standing"
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken(" under", 100, 400, 0.9),
					makeToken("standing", 400, 800, 0.85),
					makeToken(" the", 800, 1000, 0.95),
				},
			},
		},
	}

	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	if len(alignment.Words) != 2 {
		t.Fatalf("got %d words, want 2", len(alignment.Words))
	}

	if alignment.Words[0].Word != "understanding" {
		t.Errorf("word[0] = %q, want %q", alignment.Words[0].Word, "understanding")
	}
	if alignment.Words[0].StartMs != 100 || alignment.Words[0].EndMs != 800 {
		t.Errorf("word[0] timing = [%d, %d], want [100, 800]",
			alignment.Words[0].StartMs, alignment.Words[0].EndMs)
	}

	// Confidence should be average of the two tokens
	expectedConf := (0.9 + 0.85) / 2.0
	if diff := alignment.Words[0].Confidence - expectedConf; diff > 0.01 || diff < -0.01 {
		t.Errorf("word[0] confidence = %.3f, want %.3f", alignment.Words[0].Confidence, expectedConf)
	}
}

func TestParseWhisperJSON_SkipsSpecialTokens(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken("[_BEG_]", 0, 0, 1.0),
					makeToken(" Hello", 0, 500, 0.95),
					makeToken("[_EOT_]", 0, 0, 1.0),
				},
			},
		},
	}

	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	if len(alignment.Words) != 1 {
		t.Fatalf("got %d words, want 1 (special tokens should be skipped)", len(alignment.Words))
	}
}

func TestParseWhisperJSON_SkipsLowConfidence(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken(" Hello", 0, 500, 0.95),
					makeToken(" noise", 500, 700, 0.1),
					makeToken(" world", 700, 1200, 0.90),
				},
			},
		},
	}

	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	if len(alignment.Words) != 2 {
		t.Fatalf("got %d words, want 2 (low-confidence token should be skipped)", len(alignment.Words))
	}
}

func TestParseWhisperJSON_GapFill(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken(" first", 100, 100, 0.9),  // zero-duration
					makeToken(" second", 200, 500, 0.9),
				},
			},
		},
	}

	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	if len(alignment.Words) != 2 {
		t.Fatalf("got %d words, want 2", len(alignment.Words))
	}

	// Gap-fill: EndMs should be min(next_word.StartMs, StartMs+300) = min(200, 400) = 200
	if alignment.Words[0].EndMs != 200 {
		t.Errorf("gap-filled word[0].EndMs = %d, want 200", alignment.Words[0].EndMs)
	}
}

func TestParseWhisperJSON_GapFillLastWord(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken(" word", 100, 100, 0.9), // zero-duration, last word
				},
			},
		},
	}

	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	// Last word gap-fill: EndMs = StartMs + 300 = 400
	if alignment.Words[0].EndMs != 400 {
		t.Errorf("gap-filled last word EndMs = %d, want 400", alignment.Words[0].EndMs)
	}
}

func TestParseWhisperJSON_MultipleSegments(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{
			{
				Tokens: []whisperTokenRaw{
					makeToken(" Hello", 0, 500, 0.95),
				},
			},
			{
				Tokens: []whisperTokenRaw{
					makeToken(" world", 600, 1100, 0.90),
				},
			},
		},
	}

	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}

	if len(alignment.Words) != 2 {
		t.Fatalf("got %d words, want 2 (across segments)", len(alignment.Words))
	}
}

func TestParseWhisperJSON_EmptyInput(t *testing.T) {
	raw := whisperFullJSONRaw{
		Transcription: []whisperSegmentRaw{},
	}
	data, _ := json.Marshal(raw)
	alignment, err := parseWhisperJSON(data)
	if err != nil {
		t.Fatalf("parseWhisperJSON: %v", err)
	}
	if len(alignment.Words) != 0 {
		t.Errorf("got %d words, want 0", len(alignment.Words))
	}
}
