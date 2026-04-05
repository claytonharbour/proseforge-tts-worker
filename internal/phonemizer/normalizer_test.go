//go:build unit

package phonemizer

import (
	"testing"
)

func TestNormalizeNumbers(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"42", "forty two"},
		{"0", "zero"},
		{"100", "one hundred"},
		{"1000", "one thousand"},
		{"1234", "one thousand two hundred thirty four"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeDecimals(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"3.14", "three point one four"},
		{"0.5", "zero point five"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeCurrency(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"$100", "one hundred dollars"},
		{"$1", "one dollar"},
		{"$10.50", "ten dollars and fifty cents"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePercent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"14%", "fourteen percent"},
		{"100%", "one hundred percent"},
		{"by 14%.", "by fourteen percent."},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeAlphaNumeric(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Q3", "Q three"},
		{"B2 bomber", "B two bomber"},
		{"F16", "F sixteen"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeAbbreviations(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Dr. Smith", "doctor Smith"},
		{"St. Louis", "saint Louis"},
		{"Mr. Jones", "mister Jones"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeOrdinals(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1st", "first"},
		{"2nd", "second"},
		{"3rd", "third"},
		{"4th", "fourth"},
		{"21st", "twenty first"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeTime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"10:30", "ten thirty"},
		{"2:00", "two o'clock"},
		{"3:05", "three oh five"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeDecades(t *testing.T) {
	got := NormalizeText("1990s")
	want := "nineteen nineties"
	if got != want {
		t.Errorf("NormalizeText(\"1990s\") = %q, want %q", got, want)
	}
}

func TestNormalizeYears(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"in 1987", "in nineteen eighty seven"},
		{"since 2024", "since twenty twenty four"},
		{"from 1900", "from nineteen hundred"},
		{"in 2000", "in two thousand"},
		{"in 2005", "in two thousand five"},
		{"in 1905", "in nineteen oh five"},
		{"year 1776", "year seventeen seventy six"},
		{"during 1812", "during eighteen twelve"},
		// Bare numbers without context stay cardinal
		{"1987", "one thousand nine hundred eighty seven"},
		{"1234", "one thousand two hundred thirty four"},
		// Decades still work
		{"1990s", "nineteen nineties"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeDottedAbbreviations(t *testing.T) {
	got := NormalizeText("U.S.A.")
	want := "U S A"
	if got != want {
		t.Errorf("NormalizeText(\"U.S.A.\") = %q, want %q", got, want)
	}
}

func TestNormalizeCurlyApostrophes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"she\u2019d gone", "she'd gone"},
		{"it\u2018s fine", "it's fine"},
		{"don\u2019t stop", "don't stop"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeMarkdownStripping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"*italic text*", "italic text"},
		{"**bold text**", "bold text"},
		{"***bold italic***", "bold italic"},
		{"_italic text_", "italic text"},
		{"__bold text__", "bold text"},
		{"read: *Q3 exceeded fourteen percent.* She stared", "read: Q three exceeded fourteen percent. She stared"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePunctuation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello—world", "Hello — world"},
		{"Hello–world", "Hello — world"},
		{"Wait...", "Wait …"},
		{"Wait…", "Wait …"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeParagraphBreaks(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello.\n\nWorld.", "Hello. World."},
		{"A\n\nB\n\nC", "A. B. C"},
		{"No breaks here", "No breaks here"},
		{"Single\nnewline", "Single newline"},
	}

	for _, tt := range tests {
		got := NormalizeText(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSplitParagraphs(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"A\n\nB\n\nC", 3},
		{"No breaks", 1},
		{"", 0},
		{"A\nB", 1},
		{"  \n\n  ", 0},
		{"First paragraph.\n\nSecond paragraph.\n\nThird.", 3},
		{"Leading\n\n\n\nTrailing", 2}, // multiple blank lines still one split
	}

	for _, tt := range tests {
		got := SplitParagraphs(tt.input)
		if len(got) != tt.want {
			t.Errorf("SplitParagraphs(%q) = %d paragraphs %v, want %d", tt.input, len(got), got, tt.want)
		}
	}

	// Verify content of a multi-paragraph split
	parts := SplitParagraphs("Hello world.\n\nSecond part.\n\nThird part.")
	if len(parts) != 3 {
		t.Fatalf("expected 3 paragraphs, got %d", len(parts))
	}
	if parts[0] != "Hello world." {
		t.Errorf("paragraph 0 = %q, want %q", parts[0], "Hello world.")
	}
	if parts[1] != "Second part." {
		t.Errorf("paragraph 1 = %q, want %q", parts[1], "Second part.")
	}
	if parts[2] != "Third part." {
		t.Errorf("paragraph 2 = %q, want %q", parts[2], "Third part.")
	}
}

func TestNumberToWords(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "zero"},
		{1, "one"},
		{13, "thirteen"},
		{20, "twenty"},
		{42, "forty two"},
		{100, "one hundred"},
		{101, "one hundred one"},
		{999, "nine hundred ninety nine"},
		{1000, "one thousand"},
		{1001, "one thousand one"},
		{10000, "ten thousand"},
		{1000000, "one million"},
	}

	for _, tt := range tests {
		got := numberToWords(tt.n)
		if got != tt.want {
			t.Errorf("numberToWords(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
