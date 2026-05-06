package filter

import (
	"regexp"
	"strings"
	"sync"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
)

var (
	globRegexCache sync.Map // map[string]*regexp.Regexp
)

// globToRegex compiles a shell-style glob into a regexp anchored at both ends.
// Unlike path.Match / filepath.Match, '*' here matches any character including '/',
// which is what users expect when filtering URLs and HTML content.
func globToRegex(pattern string) *regexp.Regexp {
	if cached, ok := globRegexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp)
	}
	var b strings.Builder
	b.WriteByte('^')
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteByte('$')
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	globRegexCache.Store(pattern, re)
	return re
}

func matches(pattern, value string) bool {
	lower := strings.ToLower(value)
	lowerPattern := strings.ToLower(pattern)
	if strings.ContainsAny(pattern, "*?") {
		re := globToRegex(lowerPattern)
		return re != nil && re.MatchString(lower)
	}
	return strings.Contains(lower, lowerPattern)
}

func fieldValue(a *article.Article, field filterrule.MatchField) string {
	switch field {
	case filterrule.MatchFieldTitle:
		return a.Title
	case filterrule.MatchFieldURL:
		return a.URL
	case filterrule.MatchFieldContent:
		return a.Content
	case filterrule.MatchFieldDescription:
		return a.Description
	default:
		return ""
	}
}

// Apply returns true if the article should be filtered out (i.e., not shown).
func Apply(a *article.Article, rules []*filterrule.FilterRule) bool {
	if len(rules) == 0 {
		return false
	}

	var whitelists, blacklists []*filterrule.FilterRule
	for _, r := range rules {
		switch r.RuleType {
		case filterrule.RuleTypeWhitelist:
			whitelists = append(whitelists, r)
		case filterrule.RuleTypeBlacklist:
			blacklists = append(blacklists, r)
		}
	}

	blacklisted := false
	for _, r := range blacklists {
		if matches(r.MatchPattern, fieldValue(a, r.MatchField)) {
			blacklisted = true
			break
		}
	}

	whitelisted := false
	for _, r := range whitelists {
		if matches(r.MatchPattern, fieldValue(a, r.MatchField)) {
			whitelisted = true
			break
		}
	}

	// Only whitelist rules → keep if ANY whitelist matches
	if len(whitelists) > 0 && len(blacklists) == 0 {
		return !whitelisted
	}
	// Only blacklist rules → filter out if blacklisted
	if len(blacklists) > 0 && len(whitelists) == 0 {
		return blacklisted
	}
	// Mixed → filter out if blacklisted AND no whitelist rescues
	return blacklisted && !whitelisted
}
