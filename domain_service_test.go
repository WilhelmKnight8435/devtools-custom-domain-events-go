package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDomains struct{ adds int }

func (f *fakeDomains) addDomain(_ context.Context, _ string) (domainData, error) {
	f.adds++
	return domainData{ZoneID: "zone-1"}, nil
}
func (f *fakeDomains) getDomain(_ context.Context, _ string) (domainData, error) {
	return domainData{ZoneID: "zone-1", Status: "verified"}, nil
}
func (f *fakeDomains) verifyDomain(context.Context, string) error { return nil }

func TestReleaseDecision(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		allowed      bool
	}{
		{"passed build", "passed", true}, {"failed build", "failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeDomains{}
			s := newService(api, "test-secret")
			s.builds["b1"] = build{ID: "b1", Result: tc.result}
			r, err := s.publish(context.Background(), "r1", "b1", "docs.example.com")
			if (err == nil) != tc.allowed {
				t.Fatalf("publish error = %v", err)
			}
			if !tc.allowed {
				if api.adds != 0 {
					t.Fatal("domain registered for failed build")
				}
				return
			}
			if r.State != "awaiting_verification" || r.ZoneID != "zone-1" {
				t.Fatalf("release = %+v", r)
			}
			body := `{"domain":"docs.example.com"}`
			mac := hmac.New(sha256.New, []byte("test-secret"))
			mac.Write([]byte(body))
			req := httptest.NewRequest(http.MethodPost, "/webhooks/domain", strings.NewReader(body))
			req.Header.Set("X-Infrai-Signature", hex.EncodeToString(mac.Sum(nil)))
			response := httptest.NewRecorder()
			s.handler().ServeHTTP(response, req)
			if response.Code != 204 || s.releases["r1"].State != "ready" {
				t.Fatalf("response = %d; release = %+v", response.Code, s.releases["r1"])
			}
			if api.adds != 1 {
				t.Fatalf("domain registrations = %d", api.adds)
			}
		})
	}
}
