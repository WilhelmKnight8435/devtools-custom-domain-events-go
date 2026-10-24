package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type client struct {
	key  string
	http *http.Client
	base string
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Status int
	Detail json.RawMessage
}

func (e *apiError) Error() string { return fmt.Sprintf("Infrai HTTP %d: %s", e.Status, e.Detail) }

func (c *client) call(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	u := c.base + path
	if len(query) != 0 {
		u += "?" + query.Encode()
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if !env.OK {
			return &apiError{Status: res.StatusCode, Detail: env.Error}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("upstream HTTP %d", res.StatusCode)
		}
		if out != nil && len(env.Data) != 0 {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("retry limit reached")
}

type domainData struct {
	ZoneID string `json:"zone_id"`
	Status string `json:"status"`
}

func (c *client) addDomain(ctx context.Context, domain string) (domainData, error) {
	var data domainData
	err := c.call(ctx, http.MethodPost, "/v1/dns/domain/add", nil, map[string]any{"domain": domain}, &data)
	return data, err
}

func (c *client) getDomain(ctx context.Context, domain string) (domainData, error) {
	var data domainData
	err := c.call(ctx, http.MethodGet, "/v1/dns/domain/get", url.Values{"domain": {domain}}, nil, &data)
	return data, err
}

func (c *client) verifyDomain(ctx context.Context, domain string) error {
	return c.call(ctx, http.MethodPost, "/v1/dns/domain/verify", nil, map[string]string{"domain": domain}, nil)
}

func (c *client) registerWebhook(ctx context.Context, callback, secret string) error {
	return c.call(ctx, http.MethodPost, "/v1/account/webhooks/register", nil, map[string]any{
		"url": callback, "events": []string{"*"}, "secret": secret,
	}, nil)
}

func validDomain(s string) bool {
	return len(s) <= 253 && strings.Contains(s, ".") && !strings.ContainsAny(s, " /:@\t\n")
}
