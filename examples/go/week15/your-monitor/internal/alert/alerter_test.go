package alert

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week15/your-monitor/internal/db"
	"fsp/examples/week15/your-monitor/internal/dbtest"
)

// recorder is a Notifier that keeps every event it gets. With fail set,
// it refuses every event, like a chat service that is down.
type recorder struct {
	mu     sync.Mutex
	events []Event
	fail   bool
	delay  time.Duration
}

func (r *recorder) Notify(ctx context.Context, e Event) error {
	time.Sleep(r.delay)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("chat service is down")
	}
	r.events = append(r.events, e)
	return nil
}

func (r *recorder) kinds() []Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ks []Kind
	for _, e := range r.events {
		ks = append(ks, e.Kind)
	}
	return ks
}

var t0 = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

// setup adds one monitor and returns an alerter for it.
func setup(t *testing.T, n Notifier) (*pgxpool.Pool, *Alerter) {
	db0 := dbtest.New(t)
	dbtest.Exec(t, db0, `INSERT INTO monitors (url, interval_seconds) VALUES ('https://example.com', 60)`)
	return db0, &Alerter{DB: db0, Results: db.NewResultStore(db0), Notifier: n, Threshold: 3}
}

// addResult stores one result for monitor 1, minute minutes after t0.
func addResult(t *testing.T, pool *pgxpool.Pool, minute int, up bool, status int) {
	t.Helper()
	dbtest.Exec(t, pool, `INSERT INTO check_results (monitor_id, checked_at, up, status_code) VALUES (1, $1, $2, $3)`,
		t0.Add(time.Duration(minute)*time.Minute), up, status)
}

// region: alerter-test

func TestAlerterOpensAfterThreeFailuresAndResolvesOnSuccess(t *testing.T) {
	rec := &recorder{}
	pool, a := setup(t, rec)
	ctx := context.Background()
	round := func() {
		t.Helper()
		if err := a.Round(ctx); err != nil {
			t.Fatal(err)
		}
	}

	addResult(t, pool, 0, true, 200)
	addResult(t, pool, 1, false, 503)
	addResult(t, pool, 2, false, 503)
	round()
	if len(rec.kinds()) != 0 {
		t.Fatalf("two failures sent %v, want nothing", rec.kinds())
	}

	addResult(t, pool, 3, false, 502)
	round()
	round() // a second round must not open a second incident
	if got := rec.kinds(); len(got) != 1 || got[0] != Opened {
		t.Fatalf("three failures sent %v, want one opened event", got)
	}
	opened := rec.events[0]
	if opened.Cause != "status 502" || !opened.At.Equal(t0.Add(time.Minute)) {
		t.Errorf("opened event has cause %q at %v, want status 502 at the first failure, %v",
			opened.Cause, opened.At, t0.Add(time.Minute))
	}
	open, err := Store{DB: pool}.List(ctx, true)
	if err != nil || len(open) != 1 || open[0].ResolvedAt != nil {
		t.Fatalf("open incidents = %+v, %v; want one", open, err)
	}

	addResult(t, pool, 4, true, 200)
	round()
	if got := rec.kinds(); len(got) != 2 || got[1] != Resolved || rec.events[1].IncidentID != opened.IncidentID {
		t.Fatalf("after a success, events are %v, want opened then resolved for incident %d", got, opened.IncidentID)
	}
	if open, _ := (Store{DB: pool}).List(ctx, true); len(open) != 0 {
		t.Errorf("%d incidents still open, want 0", len(open))
	}
	all, _ := Store{DB: pool}.List(ctx, false)
	if len(all) != 1 || all[0].ResolvedAt == nil || !all[0].ResolvedAt.Equal(t0.Add(4*time.Minute)) {
		t.Errorf("all incidents = %+v, want one resolved at the successful check", all)
	}
}

// endregion: alerter-test

// region: retry-test

func TestFailedNotificationIsRetriedNextRound(t *testing.T) {
	rec := &recorder{fail: true}
	pool, a := setup(t, rec)
	for m := range 3 {
		addResult(t, pool, m, false, 500)
	}

	if err := a.Round(context.Background()); err == nil {
		t.Fatal("Round returned no error although the notification failed")
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM incidents`); n != 0 {
		t.Fatalf("%d incidents stored, want 0: no incident without its notification", n)
	}

	rec.fail = false
	if err := a.Round(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rec.kinds(); len(got) != 1 || got[0] != Opened {
		t.Errorf("events after the retry: %v, want one opened event", got)
	}
}

// endregion: retry-test

// region: two-alerters-test

func TestTwoAlertersNotifyOnce(t *testing.T) {
	rec := &recorder{delay: 100 * time.Millisecond} // a slow chat service widens the race
	pool, a := setup(t, rec)
	b := *a // a second alerter on the same database
	for m := range 3 {
		addResult(t, pool, m, false, 500)
	}

	var wg sync.WaitGroup
	for _, al := range []*Alerter{a, &b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := al.Round(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := rec.kinds(); len(got) != 1 {
		t.Errorf("two alerters sent %v, want one opened event", got)
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM incidents`); n != 1 {
		t.Errorf("%d incidents, want 1", n)
	}
}

// endregion: two-alerters-test
