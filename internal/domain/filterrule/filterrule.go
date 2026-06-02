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

// Valid reports whether t is a recognised rule type.
func (t RuleType) Valid() bool {
	return t == RuleTypeWhitelist || t == RuleTypeBlacklist
}

// Valid reports whether f is a recognised match field.
func (f MatchField) Valid() bool {
	switch f {
	case MatchFieldTitle, MatchFieldURL, MatchFieldContent, MatchFieldDescription:
		return true
	default:
		return false
	}
}

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
	// ListByFeedIDs loads rules for many feeds in a single query (used by export).
	ListByFeedIDs(ctx context.Context, feedIDs []string) ([]*FilterRule, error)
	// Delete removes a rule only if it belongs to the specified feed.
	Delete(ctx context.Context, feedID, ruleID string) error
	// DeleteAllByFeedID removes all rules for a feed (used during import).
	DeleteAllByFeedID(ctx context.Context, feedID string) error
}
