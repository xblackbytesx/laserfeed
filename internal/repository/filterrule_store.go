package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
)

type FilterRuleStore struct {
	db *pgxpool.Pool

	// cache holds per-feed rule lists. Filter rules change rarely but
	// ListByFeedID is called on every poll cycle, so caching avoids a query
	// per feed per poll. All mutations go through this store, so we can
	// invalidate precisely. Cached slices are treated as read-only by callers.
	mu    sync.RWMutex
	cache map[string][]*filterrule.FilterRule
}

func NewFilterRuleStore(db *pgxpool.Pool) *FilterRuleStore {
	return &FilterRuleStore{
		db:    db,
		cache: make(map[string][]*filterrule.FilterRule),
	}
}

func (s *FilterRuleStore) invalidate(feedID string) {
	s.mu.Lock()
	delete(s.cache, feedID)
	s.mu.Unlock()
}

func scanRule(row interface{ Scan(...any) error }) (*filterrule.FilterRule, error) {
	r := &filterrule.FilterRule{}
	var ruleTypeStr, matchFieldStr string
	err := row.Scan(&r.ID, &r.FeedID, &ruleTypeStr, &matchFieldStr, &r.MatchPattern, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.RuleType = filterrule.RuleType(ruleTypeStr)
	r.MatchField = filterrule.MatchField(matchFieldStr)
	return r, nil
}

func (s *FilterRuleStore) Create(ctx context.Context, r *filterrule.FilterRule) (*filterrule.FilterRule, error) {
	row := s.db.QueryRow(ctx,
		`INSERT INTO feed_filter_rules (feed_id, rule_type, match_field, match_pattern)
		VALUES ($1,$2,$3,$4)
		RETURNING id, feed_id, rule_type, match_field, match_pattern, created_at`,
		r.FeedID, string(r.RuleType), string(r.MatchField), r.MatchPattern,
	)
	created, err := scanRule(row)
	if err != nil {
		return nil, fmt.Errorf("create filter rule: %w", err)
	}
	s.invalidate(r.FeedID)
	return created, nil
}

func (s *FilterRuleStore) ListByFeedID(ctx context.Context, feedID string) ([]*filterrule.FilterRule, error) {
	s.mu.RLock()
	cached, ok := s.cache[feedID]
	s.mu.RUnlock()
	if ok {
		return cached, nil
	}

	rules, err := s.queryByFeedID(ctx, feedID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[feedID] = rules
	s.mu.Unlock()
	return rules, nil
}

func (s *FilterRuleStore) queryByFeedID(ctx context.Context, feedID string) ([]*filterrule.FilterRule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, feed_id, rule_type, match_field, match_pattern, created_at
		FROM feed_filter_rules WHERE feed_id=$1 ORDER BY created_at`,
		feedID,
	)
	if err != nil {
		return nil, fmt.Errorf("list filter rules: %w", err)
	}
	defer rows.Close()
	var rules []*filterrule.FilterRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan filter rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// ListByFeedIDs loads rules for many feeds in a single query (used by export).
// It bypasses the per-feed cache.
func (s *FilterRuleStore) ListByFeedIDs(ctx context.Context, feedIDs []string) ([]*filterrule.FilterRule, error) {
	if len(feedIDs) == 0 {
		return []*filterrule.FilterRule{}, nil
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, feed_id, rule_type, match_field, match_pattern, created_at
		FROM feed_filter_rules WHERE feed_id = ANY($1) ORDER BY feed_id, created_at`,
		feedIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("list filter rules by feed ids: %w", err)
	}
	defer rows.Close()
	var rules []*filterrule.FilterRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan filter rule: %w", err)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *FilterRuleStore) Delete(ctx context.Context, feedID, ruleID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM feed_filter_rules WHERE id=$1 AND feed_id=$2`,
		ruleID, feedID,
	)
	if err != nil {
		return fmt.Errorf("delete filter rule: %w", err)
	}
	s.invalidate(feedID)
	return nil
}

func (s *FilterRuleStore) DeleteAllByFeedID(ctx context.Context, feedID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM feed_filter_rules WHERE feed_id=$1`, feedID)
	if err != nil {
		return fmt.Errorf("delete all filter rules: %w", err)
	}
	s.invalidate(feedID)
	return nil
}
