package handler

import (
	"strings"
	"testing"
)

func TestClampImportedFeed(t *testing.T) {
	longUA := strings.Repeat("u", 501)
	bf := backupFeed{
		Name:                "",
		URL:                 "https://example.com/feed",
		PollIntervalSeconds: 1, // below the 60s form minimum
		ScrapeMaxAgeDays:    -3,
		RetentionMaxItems:   -1,
		RetentionMaxHours:   -1,
		UserAgent:           &longUA,
	}
	clampImportedFeed(&bf)

	if bf.Name != bf.URL {
		t.Errorf("empty name should fall back to URL, got %q", bf.Name)
	}
	if bf.PollIntervalSeconds != 3600 {
		t.Errorf("sub-minimum interval should clamp to 3600, got %d", bf.PollIntervalSeconds)
	}
	if bf.ScrapeMaxAgeDays != 0 || bf.RetentionMaxItems != 0 || bf.RetentionMaxHours != 0 {
		t.Errorf("negative scalars not clamped: %d/%d/%d", bf.ScrapeMaxAgeDays, bf.RetentionMaxItems, bf.RetentionMaxHours)
	}
	if bf.UserAgent != nil {
		t.Error("over-length user agent should be dropped")
	}
}

func TestClampImportedFeedKeepsValidValues(t *testing.T) {
	ua := "Mozilla/5.0"
	ph := "https://example.com/placeholder.png"
	bf := backupFeed{
		Name:                "My Feed",
		URL:                 "https://example.com/feed",
		PollIntervalSeconds: 900,
		UserAgent:           &ua,
		PlaceholderImageURL: &ph,
	}
	clampImportedFeed(&bf)

	if bf.Name != "My Feed" || bf.PollIntervalSeconds != 900 {
		t.Errorf("valid values changed: %q %d", bf.Name, bf.PollIntervalSeconds)
	}
	if bf.UserAgent == nil || *bf.UserAgent != ua {
		t.Error("valid user agent dropped")
	}
	if bf.PlaceholderImageURL == nil {
		t.Error("valid placeholder URL dropped")
	}
}

func TestClampImportedFeedDropsInvalidPlaceholder(t *testing.T) {
	bad := "javascript:alert(1)"
	bf := backupFeed{URL: "https://example.com/feed", Name: "x", PollIntervalSeconds: 900, PlaceholderImageURL: &bad}
	clampImportedFeed(&bf)
	if bf.PlaceholderImageURL != nil {
		t.Error("invalid placeholder URL should be dropped")
	}
}
