package filterrule

import "testing"

func TestRuleTypeValid(t *testing.T) {
	for _, rt := range []RuleType{RuleTypeWhitelist, RuleTypeBlacklist} {
		if !rt.Valid() {
			t.Errorf("RuleType(%q).Valid() = false, want true", rt)
		}
	}
	for _, rt := range []RuleType{"", "allow", "deny"} {
		if rt.Valid() {
			t.Errorf("RuleType(%q).Valid() = true, want false", rt)
		}
	}
}

func TestMatchFieldValid(t *testing.T) {
	for _, mf := range []MatchField{MatchFieldTitle, MatchFieldURL, MatchFieldContent, MatchFieldDescription} {
		if !mf.Valid() {
			t.Errorf("MatchField(%q).Valid() = false, want true", mf)
		}
	}
	for _, mf := range []MatchField{"", "body", "author"} {
		if mf.Valid() {
			t.Errorf("MatchField(%q).Valid() = true, want false", mf)
		}
	}
}
