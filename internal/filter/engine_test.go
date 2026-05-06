package filter

import (
	"testing"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
)

func TestMatchesContains(t *testing.T) {
	tests := []struct {
		pattern, value string
		want           bool
	}{
		{"hello", "Hello, world!", true},
		{"WORLD", "Hello, world!", true},
		{"absent", "Hello, world!", false},
		{"", "anything", true}, // empty pattern matches everything via Contains
	}
	for _, tt := range tests {
		if got := matches(tt.pattern, tt.value); got != tt.want {
			t.Errorf("matches(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}

// TestMatchesGlob_AcrossSlash is the regression test for the filepath.Match bug:
// shell-style '*' must match across '/' so URL filters work as users expect.
func TestMatchesGlob_AcrossSlash(t *testing.T) {
	tests := []struct {
		pattern, value string
		want           bool
	}{
		{"*example.com*", "https://example.com/path/to/article", true},
		{"https://*", "https://news.ycombinator.com/", true},
		{"*ycombinator*", "https://news.ycombinator.com/item?id=1", true},
		{"*.example.com/*", "https://blog.example.com/post", true},
		{"*?d=1", "/path?id=1", true},
		{"foo*bar", "foo-x-bar", true},
		{"foo*bar", "fooXbar", true},
		{"foo*bar", "no-match", false},
		{"https://blocked.example/*", "https://allowed.example/x", false},
	}
	for _, tt := range tests {
		if got := matches(tt.pattern, tt.value); got != tt.want {
			t.Errorf("matches(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}

func TestApply_OnlyBlacklist(t *testing.T) {
	a := &article.Article{Title: "Ask HN: Anyone hiring?"}
	rules := []*filterrule.FilterRule{
		{RuleType: filterrule.RuleTypeBlacklist, MatchField: filterrule.MatchFieldTitle, MatchPattern: "ask hn"},
	}
	if !Apply(a, rules) {
		t.Error("expected blacklisted article to be filtered out")
	}

	b := &article.Article{Title: "Show HN: A new tool"}
	if Apply(b, rules) {
		t.Error("expected non-matching article to pass")
	}
}

func TestApply_OnlyWhitelist(t *testing.T) {
	rules := []*filterrule.FilterRule{
		{RuleType: filterrule.RuleTypeWhitelist, MatchField: filterrule.MatchFieldTitle, MatchPattern: "show hn"},
	}
	hit := &article.Article{Title: "Show HN: A new tool"}
	miss := &article.Article{Title: "Ask HN: Anyone hiring?"}

	if Apply(hit, rules) {
		t.Error("whitelisted article should not be filtered out")
	}
	if !Apply(miss, rules) {
		t.Error("non-whitelisted article should be filtered out when only whitelist rules are present")
	}
}

func TestApply_Mixed_WhitelistRescues(t *testing.T) {
	rules := []*filterrule.FilterRule{
		{RuleType: filterrule.RuleTypeBlacklist, MatchField: filterrule.MatchFieldTitle, MatchPattern: "ask hn"},
		{RuleType: filterrule.RuleTypeWhitelist, MatchField: filterrule.MatchFieldTitle, MatchPattern: "anyone hiring"},
	}
	a := &article.Article{Title: "Ask HN: Anyone hiring?"}
	if Apply(a, rules) {
		t.Error("whitelist should rescue article from blacklist match")
	}
}

func TestApply_NoRulesKeepsArticle(t *testing.T) {
	if Apply(&article.Article{Title: "x"}, nil) {
		t.Error("article should pass when no rules exist")
	}
}

func TestApply_FieldDispatch(t *testing.T) {
	a := &article.Article{
		Title:       "ok",
		URL:         "https://blocked.example/x",
		Description: "fine",
		Content:     "fine",
	}
	rules := []*filterrule.FilterRule{
		{RuleType: filterrule.RuleTypeBlacklist, MatchField: filterrule.MatchFieldURL, MatchPattern: "*blocked.example*"},
	}
	if !Apply(a, rules) {
		t.Error("URL blacklist with glob should match across '/'")
	}
}
