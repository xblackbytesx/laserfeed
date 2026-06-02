package handler

import (
	"testing"
	"time"
)

func TestAtomCacheGetSet(t *testing.T) {
	c := newAtomCache()

	if _, ok := c.get("missing"); ok {
		t.Fatal("expected miss for unknown key")
	}

	body := []byte("<feed/>")
	c.set("tech", body)
	got, ok := c.get("tech")
	if !ok {
		t.Fatal("expected hit after set")
	}
	if string(got) != string(body) {
		t.Fatalf("got %q, want %q", got, body)
	}
}

func TestAtomCacheExpiry(t *testing.T) {
	c := newAtomCache()
	// Insert an already-expired entry directly to avoid sleeping for the TTL.
	c.entries["stale"] = atomCacheEntry{
		body:    []byte("old"),
		expires: time.Now().Add(-time.Second),
	}
	if _, ok := c.get("stale"); ok {
		t.Fatal("expected expired entry to be a miss")
	}
}

func TestAtomCacheKeysDoNotCollide(t *testing.T) {
	// A channel can never have the reserved all-feed slug (slugs are
	// alphanumeric+hyphen), so the keys must differ.
	if allFeedCacheKey == "all" {
		t.Fatal("all-feed cache key collides with a valid channel slug")
	}
}
