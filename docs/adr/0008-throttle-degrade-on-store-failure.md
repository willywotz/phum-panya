# Login throttle degrades to in-process on store failure

Status: accepted

Context: ADR-0005 moved the login rate limiter into Postgres (`DBLimiter`) so
its failed-attempt counters are shared across the api replicas introduced by
ADR-0007. `DBLimiter` swallowed every gorm error: on a `Count` failure the
count stayed `0`, so `Allowed` returned `true`, and on a `Create` failure no
attempt was recorded. When Postgres was unreachable the limiter therefore
failed **open, silently** — every login was allowed, no failures were
counted, and no log or metric was produced. That window is exactly when an
attacker able to stress the database also removes the brute-force
protection, with no operator signal.

## Decision

When any Postgres call inside `DBLimiter` returns an error, that call
**degrades to a per-replica in-process `Throttle`** (same `max` and `window`)
instead of failing open. Each store error also increments the OTel counter
`login_throttle_store_error_count` (attribute `op` = `allowed`/`fail`/`reset`)
and emits a `slog` WARN `"login throttle store error"`. Detection is
per-call: each `Allowed`/`Fail`/`Reset` checks its own gorm result and
degrades only that call — no circuit breaker, no recovery state. The
`auth.Limiter` port is unchanged; degradation is internal to `DBLimiter`.

## Why

Fail-open and fail-closed are both wrong for an authentication throttle. A
silent fail-open leaves an unthrottled brute-force window during a database
outage. A fail-closed limiter turns a database blip into a total login
lockout — a self-inflicted auth outage that also amplifies the impact of the
very outage it reacts to. Degrading to the in-process `Throttle` keeps login
working and still throttled on each replica, at the cost that an attacker can
spread attempts across replicas during the outage. This mirrors the original
in-memory `Throttle` that shipped before ADR-0005, so the degraded behavior
is a known, previously-accepted state rather than new risk. The log and
metric turn the previously silent window into an observable one.

## Considered options

- **Fail-open, but log and meter it.** Rejected: cheaper, but leaves the
  unthrottled window open — it only makes the gap visible, it does not close
  it.
- **Fail-closed (block all logins while the store is down).** Rejected: a
  database blip becomes a full auth outage, and a store the attacker can
  stress becomes a denial-of-service lever against every legitimate user.
- **Add `error`/`context` to the `auth.Limiter` port and handle failure in
  the login handler.** Rejected: widens the port for one adapter's failure
  mode; the fallback belongs inside the adapter that owns the store, keeping
  the hexagonal boundary and the handler unchanged.

## Consequences

- **Login stays available and throttled during a Postgres outage**, per
  replica rather than shared. An attacker can obtain up to `max` attempts per
  replica during the outage — the accepted best-effort trade-off.
- **The degraded window is observable.** `login_throttle_store_error_count`
  and the WARN line let operators alert on and measure store failures that
  previously produced no signal.
- **No write-back on recovery.** Failures recorded in the in-process fallback
  during an outage are not replayed into Postgres; after recovery `Allowed`
  reads the clean DB count. This is inherent to degrade-to-local and matches
  the best-effort decision.
- **Single-binary / SQLite deployments are unaffected.** They already use the
  in-memory `Throttle` (ADR-0005); `DBLimiter` only runs on the Postgres
  stack.
