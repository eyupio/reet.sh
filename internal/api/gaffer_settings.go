package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// WithGafferProviders reuses the controller's stable instance key. It does
// not configure a provider or make a network request.
func WithGafferProviders(sealer *sealed.Sealer) Option {
	return func(s *server) { s.gafferSealer = sealer }
}

func (s *server) registerGafferRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{
			accounts: "Gaffer provider settings need a browser account.", signedOut: "Sign in to configure Gaffer.", readOnly: "This account is read-only, so it cannot configure or test Gaffer providers.",
		}, h)
	}
	mux.HandleFunc("GET /api/v2/gaffer/providers", access(s.gafferList))
	mux.HandleFunc("POST /api/v2/gaffer/providers", access(s.gafferSave))
	mux.HandleFunc("PUT /api/v2/gaffer/providers/{id}", access(s.gafferSave))
	mux.HandleFunc("POST /api/v2/gaffer/providers/models", access(s.gafferModels))
	mux.HandleFunc("POST /api/v2/gaffer/providers/{id}/test", access(s.gafferAction))
	mux.HandleFunc("POST /api/v2/gaffer/providers/{id}/default", access(s.gafferAction))
	mux.HandleFunc("POST /api/v2/gaffer/providers/{id}/enabled", access(s.gafferAction))
	mux.HandleFunc("DELETE /api/v2/gaffer/providers/{id}", access(s.gafferAction))
}

func (s *server) gafferPrincipal(r *http.Request) gaffer.Principal {
	role := s.role(r)
	return gaffer.Principal{UserID: s.uid(r), CanUseLocal: role == "owner" || role == "admin"}
}

func (s *server) gafferList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.gafferProviders.List(s.gafferPrincipal(r))
	if err != nil {
		gafferError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"providers": rows, "presets": gaffer.Presets(), "available": s.gafferProviders.Available(), "canUseLocal": s.gafferPrincipal(r).CanUseLocal, "maxProviders": store.MaxGafferProviders})
}

func (s *server) gafferSave(w http.ResponseWriter, r *http.Request) {
	var in gaffer.Input
	if !decodeStrictJSON(w, r, 24<<10, &in, "That Gaffer provider request is invalid or too large.") {
		return
	}
	if r.Method == http.MethodPost && (in.ID != "" || in.Revision != 0) {
		writeErr(w, 400, "A new provider must not have an existing ID or revision.")
		return
	}
	if r.Method == http.MethodPut {
		if in.ID != "" && in.ID != r.PathValue("id") {
			writeErr(w, 400, "The provider IDs do not match.")
			return
		}
		in.ID = r.PathValue("id")
		if in.Revision < 1 {
			writeErr(w, 400, "Reload this provider before saving it.")
			return
		}
	}
	p, err := s.gafferProviders.Save(s.gafferPrincipal(r), in)
	if err != nil {
		gafferError(w, err)
		return
	}
	action := "gaffer-provider-updated"
	if r.Method == http.MethodPost {
		action = "gaffer-provider-added"
	}
	// Never audit the input: it may contain a new key or a private address.
	s.eventRequest(r, action, "", map[string]any{"provider": p.ID, "kind": p.Kind, "enabled": p.Enabled})
	s.gafferList(w, r)
}

func (s *server) gafferModels(w http.ResponseWriter, r *http.Request) {
	var in gaffer.Input
	if !decodeStrictJSON(w, r, 24<<10, &in, "That Gaffer model-list request is invalid or too large.") {
		return
	}
	models, err := s.gafferProviders.Models(r.Context(), s.gafferPrincipal(r), in)
	if err != nil {
		gafferError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"models": models})
}

func (s *server) gafferAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64 `json:"revision"`
		Enabled  *bool `json:"enabled"`
	}
	if !decodeStrictJSON(w, r, 1024, &in, "That Gaffer provider action is invalid.") {
		return
	}
	if in.Revision < 1 {
		writeErr(w, 400, "Reload this provider before changing it.")
		return
	}
	p, id := s.gafferPrincipal(r), r.PathValue("id")
	var err error
	action := "gaffer-provider-tested"
	switch {
	case r.Method == http.MethodDelete:
		action = "gaffer-provider-removed"
		err = s.gafferProviders.Delete(p, id, in.Revision)
	case r.URL.Path == "/api/v2/gaffer/providers/"+id+"/default":
		action = "gaffer-provider-default-set"
		err = s.gafferProviders.SetDefault(p, id, in.Revision)
	case r.URL.Path == "/api/v2/gaffer/providers/"+id+"/enabled":
		if in.Enabled == nil {
			writeErr(w, 400, "Choose whether this provider is enabled.")
			return
		}
		action = "gaffer-provider-paused"
		if *in.Enabled {
			action = "gaffer-provider-enabled"
		}
		err = s.gafferProviders.SetEnabled(p, id, in.Revision, *in.Enabled)
	default:
		_, err = s.gafferProviders.Test(r.Context(), p, id, in.Revision)
	}
	if err != nil {
		gafferError(w, err)
		return
	}
	s.eventRequest(r, action, "", map[string]any{"provider": id})
	s.gafferList(w, r)
}

func gafferError(w http.ResponseWriter, err error) {
	var invalid *gaffer.Invalid
	var probe *gaffer.ProbeError
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "That Gaffer provider is not available.")
	case errors.Is(err, store.ErrGafferConflict), errors.Is(err, store.ErrGafferLimit), errors.Is(err, store.ErrGafferChatBusy), errors.Is(err, store.ErrGafferChatFull):
		writeErr(w, 409, err.Error())
	case errors.Is(err, store.ErrGafferName):
		writeErrCode(w, 422, "invalid_provider", "name", err.Error(), "")
	case errors.As(err, &invalid):
		writeErrCode(w, 422, "invalid_provider", invalid.Field, invalid.Message, "")
	case errors.As(err, &probe):
		writeErr(w, 502, probe.Message)
	case errors.Is(err, gaffer.ErrForbidden):
		writeErr(w, 403, err.Error())
	case errors.Is(err, gaffer.ErrBusy):
		w.Header().Set("Retry-After", "30")
		writeErr(w, 429, err.Error())
	case errors.Is(err, gaffer.ErrUnavailable):
		writeErr(w, 503, err.Error())
	default:
		writeErr(w, 500, "Gaffer could not read or save these settings just now. Try again.")
	}
}
