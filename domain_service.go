package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

type domainAPI interface {
	addDomain(context.Context, string) (domainData, error)
	getDomain(context.Context, string) (domainData, error)
	verifyDomain(context.Context, string) error
}

type build struct {
	ID     string `json:"id"`
	Result string `json:"result"`
}
type release struct {
	ID      string `json:"id"`
	BuildID string `json:"build_id"`
	Domain  string `json:"domain"`
	State   string `json:"state"`
	ZoneID  string `json:"zone_id"`
}

type service struct {
	mu       sync.Mutex
	api      domainAPI
	secret   string
	builds   map[string]build
	releases map[string]release
}

func newService(api domainAPI, secret string) *service {
	return &service{api: api, secret: secret, builds: make(map[string]build), releases: make(map[string]release)}
}

func (s *service) publish(ctx context.Context, id, buildID, domain string) (release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.releases[id]; ok {
		if existing.BuildID != buildID || existing.Domain != domain {
			return release{}, errors.New("release id already used")
		}
		return existing, nil
	}
	if s.builds[buildID].Result != "passed" {
		return release{}, errors.New("build has not passed")
	}
	if !validDomain(domain) {
		return release{}, errors.New("invalid domain")
	}
	data, err := s.api.addDomain(ctx, domain)
	if err != nil {
		return release{}, err
	}
	if err := s.api.verifyDomain(ctx, domain); err != nil {
		return release{}, err
	}
	r := release{ID: id, BuildID: buildID, Domain: domain, State: "awaiting_verification", ZoneID: data.ZoneID}
	s.releases[id] = r
	return r, nil
}

func (s *service) onVerified(ctx context.Context, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.api.getDomain(ctx, domain)
	if err != nil {
		return err
	}
	for id, r := range s.releases {
		if r.Domain == domain && r.ZoneID == data.ZoneID && strings.EqualFold(data.Status, "verified") {
			r.State = "ready"
			s.releases[id] = r
		}
	}
	return nil
}

func (s *service) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /builds", func(w http.ResponseWriter, r *http.Request) {
		var b build
		if json.NewDecoder(r.Body).Decode(&b) != nil || b.ID == "" || (b.Result != "passed" && b.Result != "failed") {
			http.Error(w, "expected id and passed/failed result", 400)
			return
		}
		s.mu.Lock()
		if old, ok := s.builds[b.ID]; ok && old != b {
			s.mu.Unlock()
			http.Error(w, "build id already used", 409)
			return
		}
		s.builds[b.ID] = b
		s.mu.Unlock()
		writeJSON(w, 200, b)
	})
	mux.HandleFunc("POST /releases", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID      string `json:"id"`
			BuildID string `json:"build_id"`
			Domain  string `json:"domain"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" || input.BuildID == "" {
			http.Error(w, "expected id, build_id and domain", 400)
			return
		}
		value, err := s.publish(r.Context(), input.ID, input.BuildID, input.Domain)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, value)
	})
	mux.HandleFunc("GET /diagnostics/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		value, ok := s.releases[r.PathValue("id")]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "release not found", 404)
			return
		}
		writeJSON(w, 200, value)
	})
	mux.HandleFunc("POST /webhooks/domain", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		mac := hmac.New(sha256.New, []byte(s.secret))
		mac.Write(body)
		signature, err := hex.DecodeString(r.Header.Get("X-Infrai-Signature"))
		if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			http.Error(w, "invalid signature", 401)
			return
		}
		var event struct {
			Domain string `json:"domain"`
		}
		if json.Unmarshal(body, &event) != nil || !validDomain(event.Domain) {
			http.Error(w, "invalid event", 400)
			return
		}
		if err := s.onVerified(r.Context(), event.Domain); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var api *apiError
	if errors.As(err, &api) {
		status := http.StatusBadGateway
		if api.Status >= 400 && api.Status < 500 {
			status = api.Status
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err.Error() == "release id already used" {
		http.Error(w, err.Error(), 409)
		return
	}
	if err.Error() == "build has not passed" || err.Error() == "invalid domain" {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Error(w, err.Error(), 502)
}
