package filter

import (
	"path/filepath"
	"strings"

	"github.com/laserfeed/laserfeed/internal/domain/article"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
)

func matches(pattern, value string) bool {
	lower := strings.ToLower(value)
	lowerPattern := strings.ToLower(pattern)
	if strings.ContainsAny(pattern, "*?") {
		matched, err := filepath.Match(lowerPattern, lower)
		return err == nil && matched
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
