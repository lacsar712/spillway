package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lacsar712/spillway/internal/app"
	"github.com/lacsar712/spillway/internal/event"
	"github.com/lacsar712/spillway/internal/plant"
	"github.com/lacsar712/spillway/internal/reservoir"
)

type API struct {
	App *app.App
}

func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/healthz", a.health)
	mux.HandleFunc("/api/v1/meta", a.meta)
	mux.HandleFunc("/api/v1/plcs", a.plcs)
	mux.HandleFunc("/api/v1/plcs/", a.plcSub)
	mux.HandleFunc("/api/v1/journal", a.journal)
	mux.HandleFunc("/api/v1/dlq", a.dlq)
	mux.HandleFunc("/api/v1/replay/", a.replay)
	mux.HandleFunc("/api/v1/commands", a.commands)
	mux.HandleFunc("/api/v1/loopback", a.App.Loop.ServeHTTP)
	mux.HandleFunc("/api/v1/loopback/recent", a.loopRecent)
	mux.HandleFunc("/api/v1/circuits", a.circuits)
	mux.HandleFunc("/api/v1/reservoir", a.reservoir)
	mux.HandleFunc("/api/v1/bays", a.bays)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       app.Version,
		"go":            runtime.Version(),
		"queue_depth":   a.App.Broker.Depth(),
		"queue_by_dest": a.App.Broker.DepthByDest(),
		"dlq":           a.App.Dead.Len(),
		"uptime_sec":    int(time.Since(a.App.Started).Seconds()),
		"ops_hint":      "dev-ops-secret",
	})
}

func (a *API) plcs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := a.App.Plants.List()
		out := make([]map[string]any, 0, len(list))
		for _, d := range list {
			out = append(out, a.App.Plants.Public(d))
		}
		writeJSON(w, http.StatusOK, map[string]any{"plcs": out})
	case http.MethodPost:
		var in plant.CreateInput
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		d, err := a.App.Plants.Create(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		a.App.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		writeJSON(w, http.StatusCreated, a.App.Plants.Public(d))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) plcSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/plcs/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "enable" && r.Method == http.MethodPost {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		d, err := a.App.Plants.SetEnabled(id, body.Enabled)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, a.App.Plants.Public(d))
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *API) journal(w http.ResponseWriter, r *http.Request) {
	dest := r.URL.Query().Get("plc_id")
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": a.App.Log.List(dest, 50),
	})
}

func (a *API) dlq(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": a.App.Dead.List()})
}

func (a *API) replay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/replay/")
	id = strings.Trim(id, "/")
	newID, err := a.App.Replay(id)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			code = http.StatusNotFound
		}
		writeErr(w, code, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"delivery_id": newID})
}

func (a *API) commands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, event.MaxBody+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(body) > event.MaxBody {
		writeErr(w, http.StatusRequestEntityTooLarge, errors.New("body too large"))
		return
	}
	res, code, err := a.App.Pipe.Handle(r.Header, body)
	if err != nil {
		writeErr(w, code, err)
		return
	}
	writeJSON(w, code, res)
}

func (a *API) loopRecent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"received": a.App.Loop.List()})
}

func (a *API) circuits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"circuits": a.App.Limits.Public()})
}

func (a *API) reservoir(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.App.Pool.Snapshot())
	case http.MethodPost:
		var in reservoir.Reading
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := a.App.Pool.Set(in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, a.App.Pool.Snapshot())
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) bays(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"bays": a.App.Bays.List()})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}
