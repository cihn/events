package repository

import (
	"context"
	"fmt"
	"time"

	"events/internal/model"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type EventRepository interface {
	InsertBatch(ctx context.Context, events []*model.Event) error
	QueryMetrics(ctx context.Context, eventName string, from, to time.Time, groupBy string) (*MetricsResult, error)
}

type MetricsResult struct {
	TotalCount  uint64
	UniqueUsers uint64
	Breakdown   []BreakdownItem
}

type BreakdownItem struct {
	Key         string
	TotalCount  uint64
	UniqueUsers uint64
}

type clickhouseRepo struct {
	conn     driver.Conn
	database string
}

func NewEventRepository(conn driver.Conn, database string) EventRepository {
	return &clickhouseRepo{conn: conn, database: database}
}

func (r *clickhouseRepo) InsertBatch(ctx context.Context, events []*model.Event) error {
	if len(events) == 0 {
		return nil
	}

	query := "INSERT INTO " + r.database + ".events (event_id, event_name, user_id, timestamp, channel, campaign_id, tags, metadata, ingested_at)"

	batch, err := r.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, e := range events {
		if err := batch.Append(
			e.EventID,
			e.EventName,
			e.UserID,
			e.Timestamp,
			e.Channel,
			e.CampaignID,
			e.Tags,
			e.Metadata,
			e.IngestedAt,
		); err != nil {
			return fmt.Errorf("append to batch: %w", err)
		}
	}

	return batch.Send()
}

func (r *clickhouseRepo) QueryMetrics(ctx context.Context, eventName string, from, to time.Time, groupBy string) (*MetricsResult, error) {
	result := &MetricsResult{}

	totalQuery := "SELECT count() AS total_count, uniqExact(user_id) AS unique_users " +
		"FROM " + r.database + ".events FINAL " +
		"WHERE event_name = ? AND timestamp >= ? AND timestamp <= ?"

	row := r.conn.QueryRow(ctx, totalQuery, eventName, from, to)
	if err := row.Scan(&result.TotalCount, &result.UniqueUsers); err != nil {
		return nil, fmt.Errorf("query totals: %w", err)
	}

	var breakdownErr error
	switch groupBy {
	case "channel":
		result.Breakdown, breakdownErr = r.queryBreakdown(ctx,
			"if(channel = '', 'unknown', channel)",
			eventName, from, to)
	case "hourly":
		result.Breakdown, breakdownErr = r.queryBreakdown(ctx,
			"formatDateTime(toStartOfHour(timestamp), '%Y-%m-%dT%H:00:00Z')",
			eventName, from, to)
	case "daily":
		result.Breakdown, breakdownErr = r.queryBreakdown(ctx,
			"formatDateTime(toStartOfDay(timestamp), '%Y-%m-%d')",
			eventName, from, to)
	}
	if breakdownErr != nil {
		return nil, breakdownErr
	}

	return result, nil
}

func (r *clickhouseRepo) queryBreakdown(ctx context.Context, keyExpr, eventName string, from, to time.Time) ([]BreakdownItem, error) {
	query := "SELECT " + keyExpr + " AS key, count() AS total_count, uniqExact(user_id) AS unique_users " +
		"FROM " + r.database + ".events FINAL " +
		"WHERE event_name = ? AND timestamp >= ? AND timestamp <= ? " +
		"GROUP BY key ORDER BY key ASC"

	rows, err := r.conn.Query(ctx, query, eventName, from, to)
	if err != nil {
		return nil, fmt.Errorf("query breakdown: %w", err)
	}
	defer rows.Close()

	var items []BreakdownItem
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Key, &item.TotalCount, &item.UniqueUsers); err != nil {
			return nil, fmt.Errorf("scan breakdown: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
