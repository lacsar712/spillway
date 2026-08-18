package accept

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/event"
	"github.com/lacsar712/spillway/internal/hashutil"
	"github.com/lacsar712/spillway/internal/headers"
	"github.com/lacsar712/spillway/internal/idempotency"
	"github.com/lacsar712/spillway/internal/idgen"
	"github.com/lacsar712/spillway/internal/ingest"
	"github.com/lacsar712/spillway/internal/interlock"
	"github.com/lacsar712/spillway/internal/job"
	"github.com/lacsar712/spillway/internal/nonce"
	"github.com/lacsar712/spillway/internal/plant"
	"github.com/lacsar712/spillway/internal/queue"
	"github.com/lacsar712/spillway/internal/route"
	"github.com/lacsar712/spillway/internal/sign"
)

type Pipeline struct {
	Clk    clock.Clock
	Window time.Duration
	Keys   *ingest.Keys
	Nonces *nonce.Book
	Idem   *idempotency.Store
	Plants *plant.Registry
	Lock   *interlock.Guard
	Broker *queue.Broker
}

type Result struct {
	CommandID   string   `json:"command_id"`
	Replay      bool     `json:"replay"`
	Matched     int      `json:"matched"`
	DeliveryIDs []string `json:"delivery_ids"`
}

func (p *Pipeline) Handle(h http.Header, body []byte) (Result, int, error) {
	in, err := headers.ParseInbound(h)
	if err != nil {
		return Result{}, http.StatusBadRequest, err
	}
	if err := hashutil.ValidIdempotencyKey(in.IdemKey); err != nil {
		return Result{}, http.StatusBadRequest, err
	}
	secrets := p.Keys.Secrets(in.SourceKey)
	if len(secrets) == 0 {
		return Result{}, http.StatusUnauthorized, fmt.Errorf("unknown operator key %q", in.SourceKey)
	}
	if err := sign.Verify(p.Clk, p.Window, secrets, sign.Headers{
		Timestamp: in.Timestamp,
		Nonce:     in.Nonce,
		Signature: in.Signature,
	}, body); err != nil {
		if errors.Is(err, sign.ErrSkew) {
			return Result{}, http.StatusBadRequest, err
		}
		return Result{}, http.StatusUnauthorized, err
	}
	env, err := event.Parse(body)
	if err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			return Result{}, http.StatusBadRequest, err
		}
		return Result{}, http.StatusUnprocessableEntity, err
	}
	if p.Lock != nil {
		if err := p.Lock.Allow(env); err != nil {
			return Result{}, http.StatusConflict, err
		}
	}
	if err := p.Nonces.CheckAndRemember(in.Nonce); err != nil {
		return Result{}, http.StatusConflict, err
	}
	now := p.Clk.Now()
	eventID := idgen.New("cmd", now)
	bodyHash := hashutil.SHA256Hex(body)
	existing, replay, err := p.Idem.Remember(in.IdemKey, bodyHash, eventID)
	if err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			return Result{}, http.StatusConflict, err
		}
		return Result{}, http.StatusBadRequest, err
	}
	if replay {
		return Result{CommandID: existing, Replay: true}, http.StatusOK, nil
	}
	matched := p.Plants.Matching(env.Type)
	plan := route.Fanout(eventID, env.Type, body, matched, now)
	ids := make([]string, 0, len(plan.Items))
	for _, item := range plan.Items {
		d, ok := p.Plants.Get(item.PLCID)
		if !ok {
			continue
		}
		p.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		p.Broker.Enqueue(job.Job{
			CommandID:  eventID,
			DeliveryID: item.DeliveryID,
			PLCID:      item.PLCID,
			Type:       env.Type,
			Body:       append([]byte(nil), body...),
			Attempt:    0,
			NotBefore:  now,
			CreatedAt:  now,
		}, d.Ordered, d.MaxInFlight)
		ids = append(ids, item.DeliveryID)
	}
	return Result{
		CommandID:   eventID,
		Matched:     len(ids),
		DeliveryIDs: ids,
	}, http.StatusAccepted, nil
}
