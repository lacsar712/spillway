package app

import (
	"context"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/lacsar712/spillway/internal/accept"
	"github.com/lacsar712/spillway/internal/backoff"
	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/config"
	"github.com/lacsar712/spillway/internal/dlq"
	"github.com/lacsar712/spillway/internal/gate"
	"github.com/lacsar712/spillway/internal/idempotency"
	"github.com/lacsar712/spillway/internal/ingest"
	"github.com/lacsar712/spillway/internal/interlock"
	"github.com/lacsar712/spillway/internal/journal"
	"github.com/lacsar712/spillway/internal/loopback"
	"github.com/lacsar712/spillway/internal/nonce"
	"github.com/lacsar712/spillway/internal/plant"
	"github.com/lacsar712/spillway/internal/plc"
	"github.com/lacsar712/spillway/internal/queue"
	"github.com/lacsar712/spillway/internal/replay"
	"github.com/lacsar712/spillway/internal/reservoir"
	"github.com/lacsar712/spillway/internal/runtime"
	"github.com/lacsar712/spillway/internal/store"
	"github.com/lacsar712/spillway/internal/worker"
)

const Version = "0.1.0"

type App struct {
	Cfg     config.Config
	Clk     clock.Clock
	Plants  *plant.Registry
	Idem    *idempotency.Store
	Nonces  *nonce.Book
	Broker  *queue.Broker
	Keys    *ingest.Keys
	Log     *journal.Log
	Dead    *dlq.Queue
	Limits  *runtime.Limits
	Loop    *loopback.Sink
	Pipe    *accept.Pipeline
	Engine  *worker.Engine
	Snap    *store.File
	Pool    *reservoir.Pool
	Bays    *gate.Catalog
	Lock    *interlock.Guard
	Started time.Time
}

func New(cfg config.Config) (*App, error) {
	clk := clock.Real{}
	a := &App{
		Cfg:     cfg,
		Clk:     clk,
		Plants:  plant.NewRegistry(clk),
		Idem:    idempotency.New(clk, cfg.IdemTTL),
		Nonces:  nonce.New(clk, cfg.Window),
		Broker:  queue.NewBroker(clk),
		Keys:    ingest.New("ops", cfg.OpsSecret),
		Log:     journal.New(500),
		Dead:    dlq.New(200),
		Limits:  runtime.NewLimits(clk),
		Loop:    loopback.New(50),
		Pool:    reservoir.New(reservoir.Default()),
		Bays:    gate.NewCatalog(nil),
		Started: time.Now(),
	}
	a.Lock = interlock.New(a.Pool, a.Bays)
	a.Pipe = &accept.Pipeline{
		Clk:    clk,
		Window: cfg.Window,
		Keys:   a.Keys,
		Nonces: a.Nonces,
		Idem:   a.Idem,
		Plants: a.Plants,
		Lock:   a.Lock,
		Broker: a.Broker,
	}
	a.Engine = worker.New(clk, a.Broker, a.Plants, a.Limits, plc.New(10*time.Second), a.Log, a.Dead, backoff.Default())
	snap, err := store.New(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	a.Snap = snap
	if s, ok, err := snap.Load(); err != nil {
		return nil, err
	} else if ok {
		store.Apply(a.deps(), s)
	}
	if len(a.Plants.List()) == 0 {
		if err := a.seedLoopback(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *App) deps() store.Deps {
	return store.Deps{
		Plants: a.Plants,
		Idem:   a.Idem,
		Nonces: a.Nonces,
		Log:    a.Log,
		Dead:   a.Dead,
		Broker: a.Broker,
		Keys:   a.Keys,
		Limits: a.Limits,
	}
}

func (a *App) seedLoopback() error {
	raw, err := url.JoinPath(a.Cfg.PublicBase, a.Cfg.LoopbackPath)
	if err != nil {
		return err
	}
	enabled := true
	d, err := a.Plants.Create(plant.CreateInput{
		Name:         "forebay-plc",
		URL:          raw,
		Secret:       "dev-loopback-secret",
		TypePrefixes: []string{""},
		Ordered:      false,
		Rate:         20,
		Burst:        20,
		MaxInFlight:  4,
		Enabled:      &enabled,
	})
	if err != nil {
		return err
	}
	a.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	return nil
}

func (a *App) StartWorkers(ctx context.Context) {
	a.Engine.Run(ctx, a.Cfg.Workers)
	go a.snapshotLoop(ctx)
}

func (a *App) snapshotLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			a.save()
			return
		case <-t.C:
			a.save()
		}
	}
}

func (a *App) save() {
	if err := a.Snap.Save(store.Capture(a.deps())); err != nil {
		log.Printf("snapshot: %v", err)
	}
}

func (a *App) Replay(deliveryID string) (string, error) {
	now := a.Clk.Now()
	if it, ok := a.Dead.Get(deliveryID); ok {
		j, err := replay.FromDLQ(it, now)
		if err != nil {
			return "", err
		}
		d, ok := a.Plants.Get(j.PLCID)
		if !ok {
			return "", os.ErrNotExist
		}
		a.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		a.Broker.Enqueue(j, d.Ordered, d.MaxInFlight)
		_, _ = a.Dead.Remove(deliveryID)
		return j.DeliveryID, nil
	}
	e, ok := a.Log.Get(deliveryID)
	if !ok {
		return "", os.ErrNotExist
	}
	j, err := replay.FromJournal(e, now)
	if err != nil {
		return "", err
	}
	d, ok := a.Plants.Get(j.PLCID)
	if !ok {
		return "", os.ErrNotExist
	}
	a.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	a.Broker.Enqueue(j, d.Ordered, d.MaxInFlight)
	return j.DeliveryID, nil
}
