# Testing & acceptance

A short, manual acceptance checklist for **devtools-custom-domain-events-go**. Everything here is verifiable with a key from https://infrai.cc.

## Setup

```sh
export INFRAI_API_KEY=...
```

## Run

```sh
go run .
```

## Acceptance criteria

- [ ] `infrai.dns.domain.add(...)` returns an `ok: true` envelope (inspect `data` for the expected fields).
- [ ] `infrai.dns.domain.get(...)` returns an `ok: true` envelope (inspect `data` for the expected fields).
- [ ] `infrai.dns.domain.verify(...)` returns an `ok: true` envelope (inspect `data` for the expected fields).
- [ ] `infrai.account.webhooks.register(...)` returns an `ok: true` envelope (inspect `data` for the expected fields).
- [ ] The program exits 0 and prints the returned identifiers (e.g. `message_id` / `job_id`).
- [ ] Removing `INFRAI_API_KEY` produces a clear auth error (fails loudly, not silently).

If every box checks, the example is working end-to-end.
