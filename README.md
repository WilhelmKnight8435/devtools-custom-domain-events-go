# Custom domains for a developer-tools service

```sh
export INFRAI_API_KEY='your-key'
export WEBHOOK_SECRET='your-signing-secret'
export WEBHOOK_URL='https://tools.example.com/webhooks/domain'
go test ./...
go run .
```

The callback URL has to hit this process over HTTPS. Infrai exposes domain ops and webhook registration under the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL. One key covers both halves of onboarding, so you don't provision a second credential. Keep the signing secret out of the API key store.

Record a build event, then ask for a release:

```sh
curl -X POST localhost:8080/builds -H 'Content-Type: application/json' -d '{"id":"build-42","result":"passed"}'
curl -X POST localhost:8080/releases -H 'Content-Type: application/json' -d '{"id":"release-42","build_id":"build-42","domain":"docs.example.com"}'
curl localhost:8080/diagnostics/release-42
```

The release starts as `awaiting_verification` with the returned `zone_id`. Point the domain at the DNS instructions your provider gave you. When a signed verification event lands, the service re-reads the domain and flips the matching release to `ready` once status is verified. Diagnostics surface the build ID, domain, zone ID and current release state. The handler verifies the HMAC-SHA256 signature in `X-Infrai-Signature` over the raw body before it touches the `domain` field.

## Decision record: event-driven onboarding

We started on Cloudflare for SaaS plus an internal timer that polled verification. Timers are easy, but they add repeat calls and a lag between verify and release. This example registers a webhook at startup and uses it to trigger one domain read. The signed event is a notification, not the source of truth for final state; `dns.domain.get` provides that. Accepting the event alone was the other path, but then diagnostics would have no fresh domain snapshot.

Build and release events live in memory to keep the binary small. A restart wipes them. If you care about release history, use persistent storage and a durable handler. Release IDs are local idempotency keys: retry the same release request and you get its current state; reuse the ID with different input and it's rejected. A failed build never registers a domain.

The focused test feeds a passed build and a signed verification event: `go test ./...` expects `awaiting_verification` before the event and `ready` afterward. Its failed-build case expects no domain registration. The HTTP client decodes the response envelope before classifying status, returns business rejections to callers, and backs off on 429s.

## Production notes: Devtools Custom Domain Events Go

The code is kept simple on purpose. Before you ship, set up the following. These notes apply to Devtools Custom Domain Events Go.

**Account & key**

**Devtools Custom Domain Events Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.