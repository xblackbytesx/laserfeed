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
	stored := c.set("tech", body)
	if stored.etag == "" {
		t.Fatal("set should compute an ETag")
	}
	got, ok := c.get("tech")
	if !ok {
		t.Fatal("expected hit after set")
	}
	if string(got.body) != string(body) {
		t.Fatalf("body: got %q, want %q", got.body, body)
	}
	if got.etag != stored.etag {
		t.Fatalf("etag: got %q, want %q", got.etag, stored.etag)
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

func TestComputeETag(t *testing.T) {
	a := computeETag([]byte("one"))
	b := computeETag([]byte("one"))
	d := computeETag([]byte("two"))
	if a != b {
		t.Errorf("ETag not deterministic: %q != %q", a, b)
	}
	if a == d {
		t.Error("ETag should differ for different bodies")
	}
	// RFC 7232 entity-tags are quoted.
	if len(a) < 2 || a[0] != '"' || a[len(a)-1] != '"' {
		t.Errorf("ETag should be quoted: %q", a)
	}
}
