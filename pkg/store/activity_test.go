package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func seedActivity(t *testing.T, s *Store, date, model string, input, output int) {
	t.Helper()
	_, err := s.db.Exec("INSERT INTO request_logs(api_key,model,input_tokens,output_tokens,error_message,created_at) VALUES(?,?,?,?,?,?)", "private-user", model, input, output, "private-log-body", date)
	if err != nil {
		t.Fatal(err)
	}
}
func backfillActivity(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.db.Exec("DELETE FROM cost_meta WHERE key='initialized'"); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateCosts(); err != nil {
		t.Fatal(err)
	}
}
func activityNow() time.Time {
	return time.Date(2026, 9, 12, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))
}

func TestUsageActivityHourlyBoundariesAndMissingDays(t *testing.T) {
	s := openTestStore(t)
	seedActivity(t, s, "2026-09-09 23:59:59", "A", 100, 1)
	seedActivity(t, s, "2026-09-10 00:00:00", "A", 10, 2)
	seedActivity(t, s, "2026-09-10 00:59:59", "B", 0, 0)
	seedActivity(t, s, "2026-09-10 23:59:59", "A", 30, 4)
	seedActivity(t, s, "2026-09-12 00:00:00", "A", 50, 6)
	backfillActivity(t, s)
	days, err := s.GetUsageActivity(context.Background(), "2026-09-10", "2026-09-11", activityNow())
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Coverage != "matched" || days[0].RawRequests != 3 || days[0].LedgerRequests != 3 {
		t.Fatalf("bad counts: %+v", days)
	}
	if len(days[0].Hours) != 2 || days[0].Hours[0].Hour != 0 || days[0].Hours[0].Requests != 2 || days[0].Hours[0].InputTokens != 10 || days[0].Hours[1].Hour != 23 {
		t.Fatalf("bad hours: %+v", days[0].Hours)
	}
	if days[1].Coverage != "matched" || len(days[1].Hours) != 0 {
		t.Fatalf("gap: %+v", days[1])
	}
	encoded, _ := json.Marshal(days)
	if strings.Contains(string(encoded), "private-") {
		t.Fatal("private fields leaked")
	}
}

func TestUsageActivityRetentionAndPerModelMismatch(t *testing.T) {
	for _, mutation := range []string{
		"DELETE FROM request_logs WHERE model='A'",
		"UPDATE request_logs SET model=CASE model WHEN 'A' THEN 'B' ELSE 'A' END",
		"UPDATE request_logs SET input_tokens=input_tokens+1",
		"UPDATE request_logs SET output_tokens=output_tokens+1",
		"UPDATE request_logs SET created_at='2026-09-10 99:00:00' WHERE model='A'",
	} {
		t.Run(mutation, func(t *testing.T) {
			s := openTestStore(t)
			seedActivity(t, s, "2026-09-10 08:00:00", "A", 10, 2)
			seedActivity(t, s, "2026-09-10 09:00:00", "B", 20, 4)
			backfillActivity(t, s)
			if _, err := s.db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			days, err := s.GetUsageActivity(context.Background(), "2026-09-10", "2026-09-10", activityNow())
			if err != nil || days[0].Coverage != "partial" {
				t.Fatalf("must not fabricate complete hours: %+v %v", days, err)
			}
		})
	}
}
func TestUsageActivityAllPriceVersionsAndClampedTokens(t *testing.T) {
	s := openTestStore(t)
	seedActivity(t, s, "2026-09-10 08:00:00", "A", -5, -2)
	seedActivity(t, s, "2026-09-10 09:00:00", "A", 20, 4)
	backfillActivity(t, s)
	if _, err := s.db.Exec("UPDATE cost_daily SET requests=1,input_tokens=0,output_tokens=0,missing_usage=1,price_version='old'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("INSERT INTO cost_daily(day,model,price_version,requests,input_tokens,output_tokens,missing_usage) VALUES('2026-09-10','A','new',1,20,4,0)"); err != nil {
		t.Fatal(err)
	}
	days, err := s.GetUsageActivity(context.Background(), "2026-09-10", "2026-09-10", activityNow())
	if err != nil || days[0].Coverage != "matched" || days[0].LedgerRequests != 2 || days[0].Hours[0].InputTokens != 0 {
		t.Fatalf("versions: %+v %v", days, err)
	}
	if _, err := s.db.Exec("UPDATE cost_daily SET missing_usage=0 WHERE price_version='old'"); err != nil {
		t.Fatal(err)
	}
	days, err = s.GetUsageActivity(context.Background(), "2026-09-10", "2026-09-10", activityNow())
	if err != nil || days[0].Coverage != "partial" {
		t.Fatalf("missing usage mismatch: %+v %v", days, err)
	}
}
func TestUsageActivityEmptyRangeAndCancellation(t *testing.T) {
	s := openTestStore(t)
	days, err := s.GetUsageActivity(context.Background(), "2024-02-28", "2024-03-01", activityNow())
	if err != nil || len(days) != 3 || days[1].Date != "2024-02-29" {
		t.Fatal(days, err)
	}
	for _, day := range days {
		if day.Coverage != "unavailable" || day.Hours == nil {
			t.Fatal(day)
		}
	}
	for _, dates := range [][2]string{{"2026-02-30", "2026-03-01"}, {"2026-09-11", "2026-09-01"}, {"2026-08-01", "2026-09-11"}, {"2026-1-01", "2026-01-02"}} {
		if _, err = s.GetUsageActivity(context.Background(), dates[0], dates[1], activityNow()); err == nil {
			t.Fatal("accepted invalid range", dates)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.GetUsageActivity(ctx, "2026-09-01", "2026-09-02", activityNow()); err == nil {
		t.Fatal("ignored cancellation")
	}
}
