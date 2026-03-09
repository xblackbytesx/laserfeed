package filterrule

import (
	"context"
	"time"
)

type RuleType string

const (
	RuleTypeWhitelist RuleType = "whitelist"
	RuleTypeBlacklist RuleType = "blacklist"
)

type MatchField string

const (
	MatchFieldTitle       MatchField = "title"
	MatchFieldURL         MatchField = "url"
	MatchFieldContent     MatchField = "content"
	MatchFieldDescription MatchField = "description"
)

type FilterRule struct {
	ID           string
	FeedID       string
	RuleType     RuleType
	MatchField   MatchField
	MatchPattern string
	CreatedAt    time.Time
}

type Repository interface {
	Create(ctx context.Context, r *FilterRule) (*FilterRule, error)
	ListByFeedID(ctx context.Context, feedID string) ([]*FilterRule, error)
	// Delete removes a rule only if it belongs to the specified feed.
	Delete(ctx context.Context, feedID, ruleID string) error
}
