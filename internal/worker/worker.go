package worker

import (
	"context"
	"log"
	"time"

	"github.com/lacsar712/spillway/internal/backoff"
	"github.com/lacsar712/spillway/internal/classify"
	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/dlq"
	"github.com/lacsar712/spillway/internal/job"
	"github.com/lacsar712/spillway/internal/journal"
	"github.com/lacsar712/spillway/internal/plant"
	"github.com/lacsar712/spillway/internal/plc"
	"github.com/lacsar712/spillway/internal/queue"
	"github.com/lacsar712/spillway/internal/runtime"
)

type Engine struct {
	clk    clock.Clock
	broker *queue.Broker
	plants *plant.Registry
	limits *runtime.Limits
	client *plc.Client
	log    *journal.Log
	dead   *dlq.Queue
	policy backoff.Policy
}

func New(clk clock.Clock, broker *queue.Broker, plants *plant.Registry, limits *runtime.Limits, client *plc.Client, jlog *journal.Log, dead *dlq.Queue, policy backoff.Policy) *Engine {
	return &Engine{
		clk:    clk,
		broker: broker,
		plants: plants,
		limits: limits,
		client: client,
		log:    jlog,
		dead:   dead,
		policy: policy.Validate(),
	}
}

func (e *Engine) Run(ctx context.Context, n int) {
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go e.loop(ctx)
	}
}

func (e *Engine) loop(ctx context.Context) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	j, ok := e.broker.Lease()
	if !ok {
		return
	}
	defer e.broker.Release(j.PLCID)
	e.handle(ctx, j)
}

func (e *Engine) handle(ctx context.Context, j job.Job) {
	dest, ok := e.plants.Get(j.PLCID)
	if !ok || !dest.Enabled {
		e.dead.Push(dlq.FromJob(j, "plc missing or disabled", e.clk.Now()))
		e.note(j, "terminal", 0, "", "plc_unavailable")
		return
	}
	e.limits.Ensure(dest.ID, dest.Rate, dest.Burst)
	br := e.limits.Breaker(dest.ID)
	dec := br.Allow()
	if !dec.Allow {
		j.NotBefore = e.clk.Now().Add(200 * time.Millisecond)
		e.broker.Enqueue(j, dest.Ordered, dest.MaxInFlight)
		e.note(j, "skipped_open", 0, "", dec.Note)
		return
	}
	wait := e.limits.Bucket(dest.ID).Take()
	if wait > 0 {
		j.NotBefore = e.clk.Now().Add(wait)
		e.broker.Enqueue(j, dest.Ordered, dest.MaxInFlight)
		e.note(j, "rate_delayed", 0, "", wait.String())
		return
	}

	j.Attempt++
	res := e.client.Post(ctx, plc.Request{
		URL:        dest.URL,
		Secret:     dest.Secret,
		CommandID:  j.CommandID,
		DeliveryID: j.DeliveryID,
		PLCID:      j.PLCID,
		Attempt:    j.Attempt,
		Body:       j.Body,
		Now:        e.clk.Now(),
	})
	kind := classify.Combine(res.Status, res.Error)
	errMsg := ""
	if res.Error != nil {
		errMsg = res.Error.Error()
	}
	e.noteWithStatus(j, kind.String(), res.Status, errMsg, res.Body)

	switch kind {
	case classify.Success:
		br.Success()
		return
	case classify.Retryable:
		br.Failure()
		if e.policy.Exhausted(j.Attempt) {
			e.dead.Push(dlq.FromJob(j, "attempts exhausted", e.clk.Now()))
			e.note(j, "terminal", res.Status, errMsg, "exhausted")
			return
		}
		delay := e.policy.Delay(j.Attempt)
		j.NotBefore = e.clk.Now().Add(delay)
		e.broker.Enqueue(j, dest.Ordered, dest.MaxInFlight)
	case classify.Terminal:
		br.Failure()
		e.dead.Push(dlq.FromJob(j, "terminal http status", e.clk.Now()))
	default:
		log.Printf("unknown classify kind for %s", j.DeliveryID)
	}
}

func (e *Engine) note(j job.Job, kind string, status int, errMsg, note string) {
	e.noteWithStatus(j, kind, status, errMsg, note)
}

func (e *Engine) noteWithStatus(j job.Job, kind string, status int, errMsg, note string) {
	if len(note) > 240 {
		note = note[:240]
	}
	e.log.Append(journal.Entry{
		At:         e.clk.Now(),
		CommandID:  j.CommandID,
		DeliveryID: j.DeliveryID,
		PLCID:      j.PLCID,
		Attempt:    j.Attempt,
		Kind:       kind,
		Status:     status,
		Error:      errMsg,
		Note:       note,
		Type:       j.Type,
		Body:       j.Body,
		ReplayOf:   j.ReplayOf,
	})
}
