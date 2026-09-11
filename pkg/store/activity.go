package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// ActivityHour contains only aggregate usage, never account IDs or log bodies.
type ActivityHour struct {
	Hour         int   `json:"hour"`
	Requests     int64 `json:"requests"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type ActivityDay struct {
	Date           string         `json:"date"`
	Coverage       string         `json:"coverage"`
	RawRequests    int64          `json:"raw_requests"`
	LedgerRequests int64          `json:"ledger_requests"`
	Hours          []ActivityHour `json:"hours"`
}

type activityTotals struct {
	requests, input, output, missing int64
}

func (n *activityTotals) add(other activityTotals) {
	n.requests += other.requests
	n.input += other.input
	n.output += other.output
	n.missing += other.missing
}

type activityEvidence struct {
	raw, ledger map[string]activityTotals
	hours       map[int]ActivityHour
	invalid     bool
}

// GetUsageActivity reads raw hours and daily ledger in one SQLite snapshot.
// Matched means equality to recorded ledger totals, not complete lifetime usage.
// Retention or a shifted date must never turn missing hourly history into zero.
func (s *Store) GetUsageActivity(ctx context.Context, from, through string, now time.Time) ([]ActivityDay, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil || start.Format("2006-01-02") != from {
		return nil, fmt.Errorf("invalid from date")
	}
	end, err := time.Parse("2006-01-02", through)
	if err != nil || end.Format("2006-01-02") != through || end.Before(start) || end.Sub(start) > 30*24*time.Hour {
		return nil, fmt.Errorf("range must contain 1 to 31 calendar days")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var first sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT MIN(day) FROM cost_daily").Scan(&first); err != nil {
		return nil, err
	}
	evidence := make(map[string]*activityEvidence)
	get := func(date string) *activityEvidence {
		if evidence[date] == nil {
			evidence[date] = &activityEvidence{raw: make(map[string]activityTotals), ledger: make(map[string]activityTotals), hours: make(map[int]ActivityHour)}
		}
		return evidence[date]
	}
	// The indexed lexical bounds apply to the existing local datetime format.
	// Noncanonical timestamps contribute to the mismatch, not invented hours.
	rows, err := tx.QueryContext(ctx, `SELECT substr(created_at,1,10), COALESCE(model,''),
		CASE WHEN length(created_at)=19 AND substr(created_at,11,1)=' ' THEN strftime('%H',created_at) END,
		COUNT(*), COALESCE(SUM(MAX(COALESCE(input_tokens,0),0)),0),
		COALESCE(SUM(MAX(COALESCE(output_tokens,0),0)),0),
		SUM(CASE WHEN COALESCE(input_tokens,0)<=0 AND COALESCE(output_tokens,0)<=0 THEN 1 ELSE 0 END)
		FROM request_logs WHERE created_at >= ? AND created_at < ? GROUP BY 1,2,3`,
		from+" 00:00:00", end.AddDate(0, 0, 1).Format("2006-01-02")+" 00:00:00")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var date, model string
		var hour sql.NullString
		var count activityTotals
		if err := rows.Scan(&date, &model, &hour, &count.requests, &count.input, &count.output, &count.missing); err != nil {
			rows.Close()
			return nil, err
		}
		e := get(date)
		n := e.raw[model]
		n.add(count)
		e.raw[model] = n
		h, parseErr := strconv.Atoi(hour.String)
		if !hour.Valid || parseErr != nil || h < 0 || h > 23 {
			e.invalid = true
			continue
		}
		bucket := e.hours[h]
		bucket.Hour = h
		bucket.Requests += count.requests
		bucket.InputTokens += count.input
		bucket.OutputTokens += count.output
		e.hours[h] = bucket
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT day,model,SUM(requests),SUM(input_tokens),SUM(output_tokens),SUM(missing_usage)
		FROM cost_daily WHERE day >= ? AND day <= ? GROUP BY day,model`, from, through)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var date, model string
		var count activityTotals
		if err := rows.Scan(&date, &model, &count.requests, &count.input, &count.output, &count.missing); err != nil {
			rows.Close()
			return nil, err
		}
		get(date).ledger[model] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result := make([]ActivityDay, 0, int(end.Sub(start)/(24*time.Hour))+1)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		e := get(date)
		entry := ActivityDay{Date: date, Coverage: "unavailable", Hours: []ActivityHour{}}
		matched := !e.invalid && first.Valid && date >= first.String && date <= now.Format("2006-01-02") && len(e.raw) == len(e.ledger)
		for model, raw := range e.raw {
			entry.RawRequests += raw.requests
			if ledger, ok := e.ledger[model]; !ok || ledger != raw {
				matched = false
			}
		}
		for _, ledger := range e.ledger {
			entry.LedgerRequests += ledger.requests
		}
		for hour := 0; hour < 24; hour++ {
			if bucket, ok := e.hours[hour]; ok {
				entry.Hours = append(entry.Hours, bucket)
			}
		}
		if matched {
			entry.Coverage = "matched"
		} else if entry.RawRequests > 0 || entry.LedgerRequests > 0 {
			entry.Coverage = "partial"
		}
		result = append(result, entry)
	}
	return result, nil
}
