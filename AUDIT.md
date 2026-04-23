# Gitwise — Code Audit

Deep review dated 2026-04-23. Six parallel agent reviews across security, handlers, services, database, git/webhook/mirror, and frontend. Findings are concise; follow file:line to the source.

---

## Areas audited

- **Security / auth**: middleware (auth, session, rate-limit, admin, body-limit, security headers), OAuth, TOTP, SSH keys, API tokens, password storage, CORS/CSRF wiring, git smart-HTTP auth
- **HTTP handlers**: `internal/api/handlers/*.go` — input validation, envelope consistency, IDOR, pagination, error handling
- **Services layer**: 21 services (activity, agent, chat, comment, commit, embedding, importer, issue, label, llm, mention, milestone, notification, org, protection, pull, repo, review, search, team, user) — error wrapping, transactions, context, concurrency, resource leaks
- **Database**: 15 migrations, `internal/database/database.go` (pool), SQL usage in services — FKs, indexes, cascade correctness, tsvector/pgvector maintenance, injection, pool sizing
- **Git / webhook / mirror / WebSocket / importer**: smart-HTTP & SSH protocol, push protection, webhook HMAC/SSRF/retry, mirror workers, importer token handling, WS origin/auth/size
- **Frontend**: `web/src/` — TanStack Query usage, Zustand stores, effect correctness, streaming/abort, XSS in markdown, a11y, code splitting, type safety

---

## CRITICAL

1. **Git smart-HTTP serves private repos unauthenticated** — `internal/git/http.go:103` (`handleUploadPack`) and `:69` (`handleInfoRefs`) don't enforce repo visibility; anyone can clone/pull private repos or enumerate refs via `info/refs?service=git-upload-pack`. Require auth + visibility check on every smart-HTTP entrypoint.

2. **Branch protection not enforced on push** — `internal/git/http.go:63` (`handleReceivePack`) runs receive-pack before any protection check; enforcement is only post-hoc in `PostReceiveHook`. Add a pre-receive hook or run protection rules inline before invoking git.

3. **PR merge is not atomic** — `internal/services/pull/service.go:411-414` performs `git.MergeBranches` then a separate DB update; a DB failure leaves the branch merged on disk but `open` in DB. Wrap in a transaction or make the flow idempotent with "merge-already-applied" detection.

4. **tsvector full-text indexes are never maintained** — `migrations/001_initial.sql:144,177` define FTS indexes on `to_tsvector(title||body)` but no trigger updates them on UPDATE; search results go stale after edits. Add trigger-maintained `tsvector` columns or switch to GENERATED stored columns.

5. **User-delete cascade is partial** — `migrations/013_user_delete_cascade.sql` cascades some FKs but not `comments.author_id`, `reviews.author_id`, `commit_metadata.author_id`. Deleting a user either fails or orphans rows. Add cascades (or explicit anonymisation) in a new migration.

6. **Rate limiter fails open on Redis outage** — `internal/middleware/ratelimit.go:40-42` passes all requests through when Redis errors. Fail closed (503) or degrade to an in-memory bucket.

7. **Login 2FA response enables user enumeration** — `internal/api/handlers/auth.go:89-92` returns `{requires_2fa: true, pending_token}` on good password + 2FA-enabled but generic error otherwise; attacker can discriminate valid accounts. Issue the same generic response; track pending 2FA via side-channel cookie.

8. **Stream fetch leaks on unmount in ChatPanel** — `web/src/components/ChatPanel.tsx:138-217` has no `AbortController`; unmount during streaming keeps updating state and holds network. Add abort + cleanup in a `useEffect`.

---

## HIGH

### Security / auth
- `internal/middleware/session.go:64` — session cookie `SameSite=Lax`; move to `Strict` (site is single-origin).
- `internal/services/user/service.go:415` (`ValidateToken`) — updates `last_used` even for failed lookups. Move update inside the success branch.
- `internal/api/handlers/auth.go:292` — pending 2FA token passed in redirect query string; ends up in referrers/logs. Use a short-lived cookie or POST.
- `internal/services/oauth/service.go:83-84` — `redirect_uri` derived from `BaseURL` with no allowlist check. Pin to a constant allowlist.
- `internal/services/webhook/service.go:761-783` — `validateURL` does DNS lookup separate from the dialer → TOCTOU rebinding to 127.0.0.1/metadata IPs. Reuse the restricted dialer (already present in `NewService`) for validation, or validate post-connect on `Control`.
- `internal/services/importer/service.go:228,347` — clone URL embeds PAT (`https://<token>@…`). If git stderr is ever logged, token leaks. Use `GIT_ASKPASS` (mirror path already does) and never log command strings.

### Handlers / input
- `internal/api/handlers/repo.go:27,110,127`, `issue.go:120`, `pull.go:120+`, `webhook.go:198` — `limit, _ := strconv.Atoi(...)` accepts negative/huge; no clamp. Factor a `parseLimit(default, max)` helper.
- `internal/api/handlers/admin.go:165` — `QueryRow(...).Scan(&repoCount)` error ignored.
- `internal/api/handlers/browse.go:88,98` — `http.NotFound` bypasses `{data,meta,errors}` envelope mandated by CLAUDE.md.
- `internal/api/handlers/org.go:224-227` — `req.Role` not validated against an enum.
- `internal/api/handlers/import.go:46,81` — async job IDs not bound to the requesting user; anyone who learns an ID can read status.
- `internal/api/handlers/search.go:36-39` — invalid `repo_id` silently falls back to global search instead of 400.
- `internal/api/handlers/notification.go:59`, `sshkey.go:82` — handler passes `(resourceID, userID)` to the service; confirm service `WHERE user_id = $2` clause (IDOR risk otherwise).

### Services
- `internal/services/importer/service.go:165,191` — goroutines spawned with `context.Background()` from the request path; won't cancel on shutdown. Use a long-lived app context.
- `internal/services/search/service.go:678-699` — `searchAll()` unbounded 5× fan-out per request; under load → N×5 goroutines. Use a bounded pool or errgroup with semaphore.
- `internal/services/notification/service.go:188-229` — preference check and insert are two statements; disable-then-create race delivers unwanted notifications. Push the check into a single UPSERT/INSERT…WHERE.
- `internal/services/review/service.go:143` + `:81` — `updateReviewSummary` errors are logged but swallowed by caller; PR review state goes stale.
- `internal/services/chat/service.go:176` uses `fmt.Printf` for errors; switch to `slog`.

### Database
- `migrations/001_initial.sql:131` — `issues.milestone_id` is nullable UUID with **no FK**. Add FK with `ON DELETE SET NULL`.
- `migrations/014_ai_framework.sql:63` — `agent_tasks.agent_id` FK lacks `ON DELETE CASCADE`.
- `internal/database/database.go:19-22` — `MaxConns=25` with webhook (10 concurrent) + importer + indexer + API can exhaust the pool. Raise to 50–100 and/or split pools for async work.
- `internal/services/embedding/service.go:130-147` — backfill logs-and-continues on failure; NULL embeddings never retried. Track failed IDs and implement backoff.

### Git / mirror / webhook / WS
- `internal/services/webhook/service.go:573-576` — Discord webhooks HMAC-sign the *transformed* payload. Sign the source event bytes for all delivery formats so receivers can verify against a canonical body.
- `internal/services/mirror/service.go:405-420` — `RunDue` spawns up to 50 unbounded goroutines with no per-sync timeout; the next tick can pile on. Add per-sync `context.WithTimeout` and cap inflight.
- `internal/websocket/handler.go:37` — after `Upgrade`, no `SetReadLimit`/`SetReadDeadline`/`SetWriteDeadline`. Client can force large frames / stall. Set sensible limits and pong handlers.
- `internal/git/service.go:33-34` — `RepoPath` takes raw `owner,name` with no validation; relies on every caller pre-validating. Validate internally or return `(string, error)`.

### Frontend
- `web/src/stores/auth.ts:57` — `as unknown as User` masks that the login response is a discriminated union (`{user}` vs `{requires_2fa, pending_token}`). Model it explicitly.
- `web/src/components/ChatPanel.tsx:164-201` — four `JSON.parse` calls inside the SSE read loop with no try/catch; a malformed chunk kills the stream.
- `web/src/pages/PullDetailPage.tsx` — `submitReview` mutation invalidates `pull-reviews`/`pull` but not `pull-comments`; inline comments go stale.
- `web/src/hooks/useNotifications.ts:90` — `connect` depends on `[isAuthenticated, queryClient]` and the effect on `[isAuthenticated, connect]` → reconnect loop on unrelated `queryClient` identity changes. Restructure with a ref.
- `web/src/pages/IssueListPage.tsx:59` — exhaustive-deps disabled without justification; effect reads `cursor` that's not in deps.

---

## MEDIUM

### Security / hardening
- `internal/services/totp/service.go:173` — document accepted drift window (go-otp default ±30s).
- `internal/services/sshkey/service.go:47-50` — no algorithm allowlist (accepts DSA etc.). Restrict to Ed25519 / ECDSA / RSA ≥ 3072.
- `internal/middleware/body_limit.go:11` — 1 GiB default body limit is excessive; make per-route (git push high, JSON low) and reduce default to ~50 MiB.
- `internal/middleware/admin.go:10-23` — admin check re-queries the user on every request; cache an `is_admin` claim on the session.

### Handlers
- `internal/api/handlers/admin.go:221-233` — ad-hoc update query builder; extract a struct-tagged builder or a small helper.
- `internal/api/handlers/webhook.go:243` — `DeliverOne` test endpoint is fire-and-forget and returns 200 before the test fires. Await the result or return a delivery ID the UI can poll.
- `internal/api/handlers/org.go:335` — `Resolve` returns different shapes per type; normalise the payload.
- `internal/api/handlers/common.go:56-69` — `decodeJSON` doesn't reject non-object JSON.

### Services
- `internal/services/pull/service.go:183,351-356` — `Merge` reads PR state then updates non-atomically. Use `UPDATE … WHERE status='open' RETURNING …`.
- `internal/services/issue/service.go:60-63` — `resolveAssignees` does N queries; batch with `WHERE username = ANY($1)`.
- `internal/services/search/service.go:81,669` — `Total` reflects the capped slice length, not a real count; paginate with explicit `COUNT(*)` or remove `Total`.
- `internal/services/notification/service.go:194` — `Create` returns `(nil,nil)` when type disabled; callers can't distinguish "inserted" from "skipped".

### Database
- `migrations/006_webhook_deliveries.sql` — no index on `event_type`; filter-by-event triggers a seq scan.
- `migrations/001_initial.sql:235` — `idx_commits_repo_date(repo_id, committed_at DESC)` — verify queries actually order DESC; otherwise drop DESC to enable backward scans.
- `migrations/008_code_index.sql:8-9` — `content TEXT` + pg_trgm index is unbounded on monorepos. Add a `CHECK (length(content) < 1_000_000)` or truncate at ingest.
- `migrations/009_oauth_accounts.sql:7` — FK to `users` lacks `ON DELETE CASCADE`.
- `internal/services/embedding/service.go:211-218` — identifier sanitizer rejects legitimate quoted identifiers; document the contract.

### Git / mirror / webhook / WS
- `internal/services/webhook/service.go:770-783` — DNS lookup has no timeout; wrap in `context.WithTimeout`.
- `internal/services/webhook/service.go:347` — `json.Marshal` of the payload is unbounded; cap payload size before dispatch and truncate commit messages/diffs.
- `internal/services/protection/service.go:32-62` — glob patterns match more broadly than users expect (`main*` → `mainline`); document or switch to exact+regex.
- `internal/websocket/hub.go` — broadcast sends under a write lock; one slow client can stall the hub. Send via a per-connection buffered channel.

### Frontend
- `web/src/components/Markdown.tsx` — `react-markdown` with `remarkGfm` only; add `rehype-sanitize` unless a strict server-side sanitiser runs.
- `web/src/lib/api.ts:27-62` — single retry, no backoff/jitter; add exponential backoff on 5xx.
- `web/src/pages/LoginPage.tsx:98-101` — swallows provider-list fetch errors silently; log and show a fallback.
- `web/src/pages/AdminPage.tsx` — only handles `isLoading`, not `error`.
- `web/src/pages/NewPullPage.tsx:45-49` — effect overwrites user's `targetBranch` selection whenever `repoQuery.data` refetches.

---

## LOW / notes

- `internal/git/service.go:166` — `AutoInit` builds a RefSpec from a validated branch; fine, but prefer `plumbing.ReferenceName` construction.
- `internal/services/webhook/service.go:302-305` — `ListDeliveries` clamps sensibly; double-check upper bound is enforced (`> 100`).
- `internal/services/comment/service.go:87-91` — `fmt.Sprintf` for sort column; uses an allowlist, but prefer a switch to an explicit column constant.
- `web/src/pages/RepoPage.tsx:223` — external link has only `rel="noreferrer"`; add `noopener`.
- `web/src/App.tsx` — no route-level code splitting; bundle grows linearly with pages. Wrap page imports in `React.lazy`.
- `web/src/pages/ImportPage.tsx:98+` — verify `setInterval` cleanup is present on unmount.
- `web/src/stores/theme.ts` — silent localStorage catch; at least `console.warn`.
- `internal/middleware/session.go:77-80` — hex-format validation is cosmetic; real check is Redis lookup (already correct).
- Positive notes: Argon2id with `subtle.ConstantTimeCompare` for passwords; API tokens hashed (SHA-256) server-side; TOTP secrets AES-GCM at rest; CORS uses exact-string allowlist; CSP `default-src 'self'`; admin routes return 404; mirror ASKPASS script uses 0700 temp dir.

---

## Suggested next steps (priority order)

1. Close the git-protocol auth gaps (#1, #2) — biggest exposure for a Git host.
2. Make PR merge, notification create, and review-summary update transactional (#3, service HIGHs).
3. Add triggers (or stored generated columns) for tsvector maintenance and ship a migration that completes the user-delete cascade (#4, #5).
4. Fix rate-limit fail-open and login enumeration (#6, #7).
5. Sweep handler `limit` parsing and envelope consistency into shared helpers.
6. Frontend: fix abort/cleanup on streaming, discriminate login response type, add sanitiser to markdown.
