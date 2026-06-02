package feed

import "testing"

func TestNormalizeImageMode(t *testing.T) {
	tests := []struct {
		in   string
		want ImageMode
	}{
		{"none", ImageModeNone},
		{"placeholder", ImageModePlaceholder},
		{"random", ImageModeRandom},
		{"builtin", ImageModeBuiltin},
		{"extract", ImageModeNone}, // retired mode maps to none
		{"", ImageModeRandom},      // unknown falls back to random
		{"bogus", ImageModeRandom},
	}
	for _, tt := range tests {
		if got := NormalizeImageMode(tt.in); got != tt.want {
			t.Errorf("NormalizeImageMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeScrapeMethod(t *testing.T) {
	tests := []struct {
		in   string
		want ScrapeMethod
	}{
		{"readability", ScrapeMethodReadability},
		{"selector", ScrapeMethodSelector},
		{"", ScrapeMethodReadability},
		{"nonsense", ScrapeMethodReadability},
	}
	for _, tt := range tests {
		if got := NormalizeScrapeMethod(tt.in); got != tt.want {
			t.Errorf("NormalizeScrapeMethod(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeSelectorType(t *testing.T) {
	tests := []struct {
		in   string
		want SelectorType
	}{
		{"css", SelectorTypeCSS},
		{"xpath", SelectorTypeXPath},
		{"", SelectorTypeCSS},
		{"weird", SelectorTypeCSS},
	}
	for _, tt := range tests {
		if got := NormalizeSelectorType(tt.in); got != tt.want {
			t.Errorf("NormalizeSelectorType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestImageModeValid(t *testing.T) {
	valid := []ImageMode{ImageModeNone, ImageModePlaceholder, ImageModeRandom, ImageModeBuiltin}
	for _, m := range valid {
		if !m.Valid() {
			t.Errorf("ImageMode(%q).Valid() = false, want true", m)
		}
	}
	for _, m := range []ImageMode{"", "extract", "bogus"} {
		if m.Valid() {
			t.Errorf("ImageMode(%q).Valid() = true, want false", m)
		}
	}
}
