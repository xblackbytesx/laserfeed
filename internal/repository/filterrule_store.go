package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/laserfeed/laserfeed/internal/domain/filterrule"
)

type FilterRuleStore struct {
	db *pgxpool.Pool
}

func NewFilterRuleStore(db *pgxpool.Pool) *FilterRuleStore {
	return &FilterRuleStore{db: db}
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
	return created, nil
}

func (s *FilterRuleStore) ListByFeedID(ctx context.Context, feedID string) ([]*filterrule.FilterRule, error) {
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
			return nil, err
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
	return nil
}

func (s *FilterRuleStore) DeleteAllByFeedID(ctx context.Context, feedID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM feed_filter_rules WHERE feed_id=$1`, feedID)
	if err != nil {
		return fmt.Errorf("delete all filter rules: %w", err)
	}
	return nil
}
