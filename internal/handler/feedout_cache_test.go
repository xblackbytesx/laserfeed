package handler

import (
	"testing"
)

func TestEtagMatches(t *testing.T) {
	etag := `"abc123"`
	tests := []struct {
		header string
		want   bool
	}{
		{`"abc123"`, true},
		{`W/"abc123"`, true},
		{`"zzz", "abc123"`, true},
		{`W/"zzz", W/"abc123"`, true},
		{`*`, true},
		{`"zzz"`, false},
		{``, false},
	}
	for _, tt := range tests {
		if got := etagMatches(tt.header, etag); got != tt.want {
			t.Errorf("etagMatches(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

func TestAtomCacheStableLastModified(t *testing.T) {
	c := newAtomCache()
	first := c.set("k", []byte("same body"))
	second := c.set("k", []byte("same body"))
	if !second.generatedAt.Equal(first.generatedAt) {
		t.Error("unchanged body should keep its generatedAt")
	}
	third := c.set("k", []byte("different body"))
	if third.generatedAt.Equal(first.generatedAt) && third.etag == first.etag {
		t.Error("changed body should get a new etag")
	}
}

func TestAtomCacheInvalidate(t *testing.T) {
	c := newAtomCache()
	c.set("a", []byte("x"))
	c.set("b", []byte("y"))
	c.invalidate("a")
	if _, ok := c.get("a"); ok {
		t.Error("invalidated entry still served")
	}
	if _, ok := c.get("b"); !ok {
		t.Error("unrelated entry dropped")
	}
	c.invalidateAll()
	if _, ok := c.get("b"); ok {
		t.Error("invalidateAll left entries behind")
	}
}
