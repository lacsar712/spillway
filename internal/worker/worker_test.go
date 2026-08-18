package worker

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/backoff"
	"github.com/lacsar712/spillway/internal/circuit"
	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/dlq"
	"github.com/lacsar712/spillway/internal/job"
	"github.com/lacsar712/spillway/internal/journal"
	"github.com/lacsar712/spillway/internal/plant"
	"github.com/lacsar712/spillway/internal/plc"
	"github.com/lacsar712/spillway/internal/queue"
	"github.com/lacsar712/spillway/internal/runtime"
)

type engineOpts struct {
	rate    float64
	burst   int
	timeout time.Duration
	circuit circuit.Settings
}

func newTestEngine(t *testing.T, url string, clk clock.Clock) (*Engine, *plant.PLC) {
	t.Helper()
	return newTestEngineOpts(t, url, clk, engineOpts{})
}

func newTestEngineOpts(t *testing.T, url string, clk clock.Clock, opts engineOpts) (*Engine, *plant.PLC) {
	t.Helper()
	if opts.rate <= 0 {
		opts.rate = 100
	}
	if opts.burst < 1 {
		opts.burst = 100
	}
	if opts.timeout <= 0 {
		opts.timeout = 2 * time.Second
	}
	if opts.circuit.FailThreshold < 1 {
		opts.circuit = circuit.DefaultSettings()
	}
	dests := plant.NewRegistry(clk)
	enabled := true
	d, err := dests.Create(plant.CreateInput{
		Name:         "t",
		URL:          url,
		Secret:       "abcdefgh",
		TypePrefixes: []string{""},
		Rate:         opts.rate,
		Burst:        opts.burst,
		MaxInFlight:  4,
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	broker := queue.NewBroker(clk)
	broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	gates := runtime.NewLimitsWithCircuit(clk, opts.circuit)
	e := New(clk, broker, dests, gates, plc.New(opts.timeout), journal.New(50), dlq.New(50), backoff.Policy{
		Base:        time.Millisecond,
		Cap:         time.Millisecond,
		MaxAttempts: 8,
	})
	return e, &d
}

func sampleJob(destID string, clk clock.Clock, deliveryID string) job.Job {
	return job.Job{
		CommandID:  "evt_test",
		DeliveryID: deliveryID,
		PLCID:      destID,
		Type:       "gate.raise",
		Body:       []byte(`{"type":"gate.raise","payload":{}}`),
		Attempt:    0,
		NotBefore:  clk.Now(),
		CreatedAt:  clk.Now(),
	}
}

func TestHandleRetriesStatus429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngine(t, srv.URL, clk)
	j := job.Job{
		CommandID:  "evt_test",
		DeliveryID: "dlv_test429",
		PLCID:      dest.ID,
		Type:       "gate.raise",
		Body:       []byte(`{"type":"gate.raise","payload":{}}`),
		Attempt:    0,
		NotBefore:  clk.Now(),
		CreatedAt:  clk.Now(),
	}
	e.handle(context.Background(), j)
	if e.dead.Len() != 0 {
		t.Fatalf("429 should not go to dlq, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("429 should requeue, depth=%d", e.broker.Depth())
	}
}

func TestHandleTreats202AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngine(t, srv.URL, clk)
	j := job.Job{
		CommandID:  "evt_test",
		DeliveryID: "dlv_test202",
		PLCID:      dest.ID,
		Type:       "gate.raise",
		Body:       []byte(`{"type":"gate.raise","payload":{}}`),
		Attempt:    0,
		NotBefore:  clk.Now(),
		CreatedAt:  clk.Now(),
	}
	e.handle(context.Background(), j)
	if e.dead.Len() != 0 {
		t.Fatalf("202 should not go to dlq, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 0 {
		t.Fatalf("202 should not requeue, depth=%d", e.broker.Depth())
	}
	entries := e.log.List("", 10)
	if len(entries) == 0 || entries[0].Kind != "success" {
		t.Fatalf("want success journal, got %+v", entries)
	}
}

func TestHandleHonorsCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c
		_, _ = io.Copy(io.Discard, c)
		_ = c.Close()
	}()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, "http://"+ln.Addr().String(), clk, engineOpts{timeout: 15 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.handle(ctx, sampleJob(dest.ID, clk, "dlv_cancel"))
	}()
	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("outbound request never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handle ignored cancel and kept the outbound request alive")
	}
}

func TestHandleTimeoutIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, srv.URL, clk, engineOpts{timeout: 50 * time.Millisecond})
	e.handle(context.Background(), sampleJob(dest.ID, clk, "dlv_timeout"))
	if e.dead.Len() != 0 {
		t.Fatalf("timeout must not go to dlq, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("timeout must requeue, depth=%d", e.broker.Depth())
	}
}

func TestHandleSuccessClearsBreaker(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 2 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, srv.URL, clk, engineOpts{
		circuit: circuit.Settings{FailThreshold: 2, OpenFor: time.Hour, Probes: 1},
	})
	e.handle(context.Background(), sampleJob(dest.ID, clk, "dlv_brk_1"))
	e.handle(context.Background(), sampleJob(dest.ID, clk, "dlv_brk_2"))
	e.handle(context.Background(), sampleJob(dest.ID, clk, "dlv_brk_3"))
	snap := e.limits.Breaker(dest.ID).Snapshot()
	if snap.State != "closed" {
		t.Fatalf("success must clear failures; breaker state=%s failures=%d", snap.State, snap.Failures)
	}
	if e.dead.Len() != 0 {
		t.Fatalf("third 500 after a success must retry, not dlq; dlq=%d", e.dead.Len())
	}
}

func TestHandleRespectsRateLimit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, srv.URL, clk, engineOpts{rate: 0.0001, burst: 1})
	e.handle(context.Background(), sampleJob(dest.ID, clk, "dlv_rl_1"))
	e.handle(context.Background(), sampleJob(dest.ID, clk, "dlv_rl_2"))
	if hits.Load() != 1 {
		t.Fatalf("burst=1 must not POST twice, hits=%d", hits.Load())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("rate-limited job must requeue, depth=%d", e.broker.Depth())
	}
}
