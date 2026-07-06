package atom

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/channel"
	"github.com/laserfeed/laserfeed/internal/domain/feed"
)

// The feed-level <updated> must come from the newest entry, not generation
// time, so repeated generations of unchanged content are byte-identical and
// downstream ETag/Last-Modified validators hold.
func TestGenerateAtomStableAcrossRegenerations(t *testing.T) {
	ch := &channel.Channel{Name: "Test", Slug: "test"}
	older := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC)
	articles := []*article.Article{
		{GUID: "https://example.com/a1", Title: "One", URL: "https://example.com/a1", PublishedAt: newest},
		{GUID: "https://example.com/a2", Title: "Two", URL: "https://example.com/a2", PublishedAt: older},
	}
	feeds := map[string]*feed.Feed{}

	first, err := GenerateAtom(ch, articles, feeds, "https://feeds.example.com")
	if err != nil {
		t.Fatalf("GenerateAtom: %v", err)
	}
	second, err := GenerateAtom(ch, articles, feeds, "https://feeds.example.com")
	if err != nil {
		t.Fatalf("GenerateAtom: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("regenerating unchanged content produced different bytes")
	}
	if !strings.Contains(string(first), "<updated>2026-07-02T12:30:00Z</updated>") {
		t.Errorf("feed updated should be the newest entry time:\n%s", first)
	}
}

func TestGenerateAtomEmptyChannelHasUpdated(t *testing.T) {
	ch := &channel.Channel{Name: "Empty", Slug: "empty"}
	out, err := GenerateAtom(ch, nil, map[string]*feed.Feed{}, "https://feeds.example.com")
	if err != nil {
		t.Fatalf("GenerateAtom: %v", err)
	}
	if !strings.Contains(string(out), "<updated>") {
		t.Error("empty feed must still carry an updated element")
	}
}
