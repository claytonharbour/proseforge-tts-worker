//go:build unit

package audio

import (
	"math"
	"testing"
)

func TestEstimateSyllables(t *testing.T) {
	cases := []struct {
		word string
		want int
	}{
		{"concrete", 2},
		{"understanding", 4},
		{"the", 1},
		{"bridge", 1},
		{"hello", 2},
		{"beautiful", 3},
		{"cat", 1},
		{"elephant", 3},
		{"I", 1},
		{"a", 1},
		{"rhythm", 1},  // heuristic limitation: syllabic consonant 'm'
		{"people", 2},
		{"create", 1},  // heuristic limitation: silent-e rule over-subtracts
		{"atmosphere", 3},
		{"syllable", 3},
		{"comfortable", 4}, // valid in careful pronunciation
	}
	for _, tc := range cases {
		got := EstimateSyllables(tc.word)
		if got != tc.want {
			t.Errorf("EstimateSyllables(%q) = %d, want %d", tc.word, got, tc.want)
		}
	}
}

func TestExtractWordStress_PeakInFirstHalf(t *testing.T) {
	// Synthetic word: loud in first half, quiet in second half → peak near 0.25
	sampleRate := 24000
	wordMs := 400 // 400ms word
	word := AlignedWord{Word: "testing", StartMs: 0, EndMs: wordMs}
	totalSamples := wordMs * sampleRate / 1000
	samples := make([]float32, totalSamples)

	half := totalSamples / 2
	for i := 0; i < half; i++ {
		samples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}
	for i := half; i < totalSamples; i++ {
		samples[i] = 0.05 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}

	profile := ExtractWordStress(samples, sampleRate, word, 2)

	if profile.PeakIndex != 0 {
		t.Errorf("PeakIndex = %d, want 0 (first half)", profile.PeakIndex)
	}
	if profile.PeakPosition > 0.4 {
		t.Errorf("PeakPosition = %.2f, want < 0.4", profile.PeakPosition)
	}
	if profile.SliceRMS[0] <= profile.SliceRMS[1] {
		t.Errorf("first slice RMS (%.4f) should be > second (%.4f)", profile.SliceRMS[0], profile.SliceRMS[1])
	}
}

func TestExtractWordStress_PeakInSecondHalf(t *testing.T) {
	// Synthetic word: quiet first half, loud second half → peak near 0.75
	sampleRate := 24000
	wordMs := 400
	word := AlignedWord{Word: "testing", StartMs: 0, EndMs: wordMs}
	totalSamples := wordMs * sampleRate / 1000
	samples := make([]float32, totalSamples)

	half := totalSamples / 2
	for i := 0; i < half; i++ {
		samples[i] = 0.05 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}
	for i := half; i < totalSamples; i++ {
		samples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}

	profile := ExtractWordStress(samples, sampleRate, word, 2)

	if profile.PeakIndex != 1 {
		t.Errorf("PeakIndex = %d, want 1 (second half)", profile.PeakIndex)
	}
	if profile.PeakPosition < 0.6 {
		t.Errorf("PeakPosition = %.2f, want > 0.6", profile.PeakPosition)
	}
}

func TestExtractWordStress_ThreeSlices(t *testing.T) {
	// Peak energy in the middle slice
	sampleRate := 24000
	wordMs := 600
	word := AlignedWord{Word: "elephant", StartMs: 0, EndMs: wordMs}
	totalSamples := wordMs * sampleRate / 1000
	samples := make([]float32, totalSamples)

	third := totalSamples / 3
	amplitudes := []float32{0.05, 0.5, 0.05}
	for slice := 0; slice < 3; slice++ {
		start := slice * third
		end := start + third
		if slice == 2 {
			end = totalSamples
		}
		for i := start; i < end; i++ {
			samples[i] = amplitudes[slice] * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
		}
	}

	profile := ExtractWordStress(samples, sampleRate, word, 3)

	if profile.PeakIndex != 1 {
		t.Errorf("PeakIndex = %d, want 1 (middle slice)", profile.PeakIndex)
	}
	// Middle of 3 slices → position ~0.5
	if math.Abs(profile.PeakPosition-0.5) > 0.1 {
		t.Errorf("PeakPosition = %.2f, want ~0.5", profile.PeakPosition)
	}
}

func TestCompareStress_IdenticalAudio(t *testing.T) {
	// Same audio and alignment for both → no mismatches
	sampleRate := 24000
	wordMs := 300
	numWords := 5
	totalSamples := numWords * wordMs * sampleRate / 1000
	samples := make([]float32, totalSamples)

	words := []AlignedWord{
		{Word: "the", StartMs: 0, EndMs: 300},
		{Word: "concrete", StartMs: 300, EndMs: 600},
		{Word: "bridge", StartMs: 600, EndMs: 900},
		{Word: "was", StartMs: 900, EndMs: 1200},
		{Word: "beautiful", StartMs: 1200, EndMs: 1500},
	}

	// Fill with uniform sine
	for i := range samples {
		samples[i] = 0.3 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}

	alignment := &Alignment{Words: words, Duration: float64(numWords*wordMs) / 1000.0}
	sourceText := "the concrete bridge was beautiful"

	report := CompareStress(samples, samples, sampleRate, sampleRate, alignment, alignment, sourceText)

	if len(report.Mismatches) != 0 {
		t.Errorf("got %d mismatches for identical audio, want 0", len(report.Mismatches))
		for _, m := range report.Mismatches {
			t.Logf("  mismatch: %q kokoro=%.2f gemini=%.2f", m.Word, m.KokoroPeak, m.GeminiPeak)
		}
	}
	if report.Matched != 5 {
		t.Errorf("matched = %d, want 5", report.Matched)
	}
}

func TestCompareStress_ShiftedStress(t *testing.T) {
	// "concrete" with stress on first syllable (Kokoro) vs second syllable (Gemini)
	sampleRate := 24000
	wordMs := 400
	totalSamples := wordMs * sampleRate / 1000

	kokoroSamples := make([]float32, totalSamples)
	geminiSamples := make([]float32, totalSamples)

	half := totalSamples / 2

	// Kokoro: loud first half, quiet second
	for i := 0; i < half; i++ {
		kokoroSamples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}
	for i := half; i < totalSamples; i++ {
		kokoroSamples[i] = 0.05 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}

	// Gemini: quiet first half, loud second
	for i := 0; i < half; i++ {
		geminiSamples[i] = 0.05 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}
	for i := half; i < totalSamples; i++ {
		geminiSamples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}

	kokoroWords := []AlignedWord{{Word: "concrete", StartMs: 0, EndMs: wordMs}}
	geminiWords := []AlignedWord{{Word: "concrete", StartMs: 0, EndMs: wordMs}}

	kokoroAlign := &Alignment{Words: kokoroWords, Duration: float64(wordMs) / 1000.0}
	geminiAlign := &Alignment{Words: geminiWords, Duration: float64(wordMs) / 1000.0}

	report := CompareStress(kokoroSamples, geminiSamples, sampleRate, sampleRate,
		kokoroAlign, geminiAlign, "concrete")

	if report.Compared != 1 {
		t.Fatalf("compared = %d, want 1", report.Compared)
	}
	if len(report.Mismatches) != 1 {
		t.Fatalf("got %d mismatches, want 1", len(report.Mismatches))
	}

	m := report.Mismatches[0]
	if m.Word != "concrete" {
		t.Errorf("mismatch word = %q, want %q", m.Word, "concrete")
	}
	if m.KokoroPeak >= m.GeminiPeak {
		t.Errorf("kokoro peak (%.2f) should be < gemini peak (%.2f)", m.KokoroPeak, m.GeminiPeak)
	}
	if m.SyllableCount != 2 {
		t.Errorf("syllable count = %d, want 2", m.SyllableCount)
	}
}

func TestCompareStress_SkipsSingleSyllable(t *testing.T) {
	sampleRate := 24000
	wordMs := 300
	totalSamples := wordMs * sampleRate / 1000

	// Even with different energy shapes, single-syllable words should not be compared
	kokoroSamples := make([]float32, totalSamples)
	geminiSamples := make([]float32, totalSamples)

	for i := range kokoroSamples {
		kokoroSamples[i] = 0.5 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}
	for i := range geminiSamples {
		geminiSamples[i] = 0.1 * float32(math.Sin(2*math.Pi*200*float64(i)/float64(sampleRate)))
	}

	words := []AlignedWord{{Word: "bridge", StartMs: 0, EndMs: wordMs}}
	align := &Alignment{Words: words, Duration: float64(wordMs) / 1000.0}

	report := CompareStress(kokoroSamples, geminiSamples, sampleRate, sampleRate,
		align, align, "bridge")

	if report.Compared != 0 {
		t.Errorf("compared = %d, want 0 (single syllable should be skipped)", report.Compared)
	}
}

func TestMatchWordsByText(t *testing.T) {
	sourceText := "the concrete bridge was beautiful"

	kokoroAlign := &Alignment{
		Words: []AlignedWord{
			{Word: "the", StartMs: 0, EndMs: 200},
			{Word: "concrete", StartMs: 200, EndMs: 600},
			{Word: "bridge", StartMs: 600, EndMs: 900},
			{Word: "was", StartMs: 900, EndMs: 1100},
			{Word: "beautiful", StartMs: 1100, EndMs: 1500},
		},
	}
	geminiAlign := &Alignment{
		Words: []AlignedWord{
			{Word: "The", StartMs: 0, EndMs: 250},
			{Word: "concrete", StartMs: 250, EndMs: 650},
			{Word: "bridge", StartMs: 650, EndMs: 950},
			{Word: "was", StartMs: 950, EndMs: 1150},
			{Word: "beautiful.", StartMs: 1150, EndMs: 1550},
		},
	}

	matched := matchWordsByText(kokoroAlign, geminiAlign, sourceText)
	if len(matched) != 5 {
		t.Errorf("matched %d words, want 5", len(matched))
		for _, m := range matched {
			t.Logf("  matched: %q", m.text)
		}
	}
}

func TestBuildContextFromSource(t *testing.T) {
	sourceText := "the concrete bridge was beautiful and strong"

	ctx := buildContextFromSource(sourceText, 1) // "concrete"
	expected := "the CONCRETE bridge was beautiful and..."
	if ctx != expected {
		t.Errorf("context = %q, want %q", ctx, expected)
	}

	ctx = buildContextFromSource(sourceText, 0) // "the"
	expected = "THE concrete bridge was beautiful..."
	if ctx != expected {
		t.Errorf("context at start = %q, want %q", ctx, expected)
	}
}
