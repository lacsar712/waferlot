package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lacsar712/waferlot/internal/fab"
	"github.com/lacsar712/waferlot/internal/lotevent"
	"github.com/lacsar712/waferlot/internal/mes"
)

type API struct {
	Plant *fab.Plant
}

func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/healthz", a.health)
	mux.HandleFunc("/api/v1/meta", a.meta)
	mux.HandleFunc("/api/v1/collectors", a.collectors)
	mux.HandleFunc("/api/v1/collectors/", a.collectorSub)
	mux.HandleFunc("GET /api/v1/runlog", a.runlog)
	mux.HandleFunc("GET /api/v1/holdbin", a.holdbin)
	mux.HandleFunc("/api/v1/reissue/", a.reissue)
	mux.HandleFunc("/api/v1/lots", a.lots)
	mux.HandleFunc("/api/v1/echo", a.Plant.Loop.ServeHTTP)
	mux.HandleFunc("GET /api/v1/echo/recent", a.echoRecent)
	mux.HandleFunc("GET /api/v1/trips", a.trips)
	mux.HandleFunc("GET /api/v1/tools", a.tools)
	mux.HandleFunc("GET /api/v1/steps", a.steps)
	mux.HandleFunc("GET /api/v1/programs", a.programs)
	mux.HandleFunc("GET /api/v1/yard", a.yard)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":            fab.Version,
		"go":                 runtime.Version(),
		"queue_depth":        a.Plant.Broker.Depth(),
		"queue_by_collector": a.Plant.Broker.DepthByCollector(),
		"holdbin":            a.Plant.Dead.Len(),
		"uptime_sec":         int(time.Since(a.Plant.Started).Seconds()),
		"ingest_hint":        "dev-tool-secret",
	})
}

func (a *API) collectors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := a.Plant.Cols.List()
		out := make([]map[string]any, 0, len(list))
		for _, d := range list {
			out = append(out, a.Plant.Cols.Public(d))
		}
		writeJSON(w, http.StatusOK, map[string]any{"collectors": out})
	case http.MethodPost:
		var in mes.CreateInput
		if err := readJSON(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		d, err := a.Plant.Cols.Create(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		a.Plant.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		writeJSON(w, http.StatusCreated, a.Plant.Cols.Public(d))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *API) collectorSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/collectors/")
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
		d, err := a.Plant.Cols.SetEnabled(id, body.Enabled)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, a.Plant.Cols.Public(d))
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *API) runlog(w http.ResponseWriter, r *http.Request) {
	col := r.URL.Query().Get("collector_id")
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": a.Plant.Log.List(col, 50),
	})
}

func (a *API) holdbin(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": a.Plant.Dead.List()})
}

func (a *API) reissue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/reissue/")
	id = strings.Trim(id, "/")
	newID, err := a.Plant.Reissue(id)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			code = http.StatusNotFound
		}
		writeErr(w, code, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"forward_id": newID})
}

func (a *API) lots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, lotevent.MaxBody+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(body) > lotevent.MaxBody {
		writeErr(w, http.StatusRequestEntityTooLarge, errors.New("body too large"))
		return
	}
	res, code, err := a.Plant.Pipe.Handle(r.Header, body)
	if err != nil {
		writeErr(w, code, err)
		return
	}
	writeJSON(w, code, res)
}

func (a *API) echoRecent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"received": a.Plant.Loop.List()})
}

func (a *API) trips(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"isolators": a.Plant.Gates.Public()})
}

func (a *API) tools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": a.Plant.Tools.List()})
}

func (a *API) steps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"steps": a.Plant.Steps.List()})
}

func (a *API) programs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"process_programs": a.Plant.Programs.List()})
}

func (a *API) yard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lots": a.Plant.Lots.List()})
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
