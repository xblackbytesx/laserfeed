package poller

import (
	"testing"

	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
	"github.com/mmcdole/gofeed"
)

func TestItemGUID(t *testing.T) {
	withGUID := &gofeed.Item{GUID: "guid-1", Link: "https://example.com/a"}
	if got := itemGUID(withGUID); got != "guid-1" {
		t.Errorf("got %q", got)
	}
	linkOnly := &gofeed.Item{Link: "https://example.com/b"}
	if got := itemGUID(linkOnly); got != "https://example.com/b" {
		t.Errorf("got %q", got)
	}
}

func TestRulesMatchContent(t *testing.T) {
	titleRule := &filterrule.FilterRule{MatchField: filterrule.MatchFieldTitle}
	contentRule := &filterrule.FilterRule{MatchField: filterrule.MatchFieldContent}

	if rulesMatchContent([]*filterrule.FilterRule{titleRule}) {
		t.Error("title-only rules should not report content matching")
	}
	if !rulesMatchContent([]*filterrule.FilterRule{titleRule, contentRule}) {
		t.Error("content rule not detected")
	}
	if rulesMatchContent(nil) {
		t.Error("nil rules should report false")
	}
}
