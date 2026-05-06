# UX gap-fix sprint — design

**Date:** 2026-05-06
**Scope:** P0 + P1 + Cmd+K search + per-page activity feed (option C from the audit
ranking, with the aesthetic-lift bucket explicitly out of scope per the project's
positron.ai / platformd visual conformance).
**Out of scope:** display-font / motion / breadcrumb redesign (P2 aesthetic),
source-link refs (P3), modal a11y (P3), product/team rename + delete (P3),
real-time updates (P4), full activity page with pagination (deferred — option C
from the activity-feed brainstorm). FTS for ACID-text search.

---

## 1. P0 — onboarding sequence

The current first-run flow dead-ends a brand-new user. They land at `/`, get
redirected to `/users/log-in`, get a magic link in their email, log in, get
dropped on `/teams` empty state, create a team, get dropped on the team page
empty state which says "create a product to get started," create a product, get
dropped on `acai push --product X --all` — but they have no CLI installed and
no API token. The order is wrong.

This sprint sequences the flow correctly via three concrete changes:

### 1.1 Magic-link confirmation page

After `POST /users/log-in` succeeds, render a new `/users/log-in/sent` page
(it can be a sub-route or a query-param state on the same handler). Surfaces:

- Echo the email entered ("we sent a sign-in link to **alex@acme.com**")
- Note the token's lifetime ("expires in 15 minutes")
- A "didn't get it?" form that re-submits the email and re-renders this same
  page on success — no separate resend endpoint needed.

### 1.2 CLI install card

A small reusable templ component, used in two places:
- The team page's "no products yet" empty state
- The tokens page's "no tokens yet" empty state

Content (markup, not literal copy — to be tightened during implementation):

```
1. install the CLI
   npm install -g @acai.sh/cli   [copy]
2. mint an API token
   <link to /t/:team/tokens>
3. push from your repo
   ACAI_API_BASE_URL=...
   ACAI_API_TOKEN=...
   acai push --product <name> --all
   [copy block]
```

Steps must be numbered; the install line and the env-var block are
copy-buttoned (see §3.3). The same component is used in both empty states so
the user sees the same sequence regardless of where they entered.

### 1.3 Token creation success page next-step block

After minting a token, the existing plaintext-token banner stays, but a
numbered next-step block is appended below it:

1. install the CLI (with copy snippet)
2. drop these env vars in your repo's `.env` (with copy snippet,
   pre-filled with the just-minted token plaintext)
3. run `acai push --product <name> --all` — list the user's products as
   selectable choices

This is a one-time block; on subsequent renders of the tokens page (without a
fresh `NewlyCreatedToken`) it does not appear.

---

## 2. P1 — operational improvements

### 2.1 Inline progress on index pages

Three pages today render bare lists:
- `/t/:team/implementations` — flat impls grouped by product
- `/t/:team/features` — features with product chips
- `/t/:team/branches` — branches with tracking-impl chips

All three get a per-row mini progress bar + compact status legend, reusing the
already-shipped `progressBar` and `statusLegend` templ helpers (defined in
`internal/site/views/feature.templ`).

For the impls index: each impl row gets the per-impl roll-up from
`services.ResolveImplOverview` (already exists). Per-row cost: 1 service call
per impl. Acceptable at typical team sizes; no new service code.

For the features index: each feature row gets the team-wide aggregate from
`services.FeatureViewService.Resolve` (already exists, returns
`ImplementationCard`s). Folding to a single aggregate per feature is in-view
arithmetic.

For the branches index: per-branch progress is more complex (a branch may
feed multiple impls, and the impl × feature roll-up is what we have). v1
treatment: skip per-branch progress, show only the existing tracking-impl
chips. Defer "branch progress" to a future sprint.

### 2.2 Inline status editing on the ACID drilldown

Each status pill on `/t/:team/i/:impl_slug/f/:feature_name` becomes a
`<select>` with the 7 valid statuses (null + assigned + blocked + incomplete
+ completed + rejected + accepted). Datastar `data-on-change` submits a small
form to a new route:

```
POST /t/:team/i/:impl_slug/f/:feature_name/acid/:acid/status
form: status=<value>, gorilla.csrf.Token=<token>
```

CSRF-protected via the existing site middleware. Handler updates the
`feature_impl_states` row (or inserts one if absent) for that
(impl, feature, ACID) tuple, then 303s back to the same page.

No optimistic update — full re-render is acceptable, the page is small.

Authorization: any team member with `member` role or higher can edit
status. (Future: per-impl ownership, deferred.)

### 2.3 Copy buttons on snippets

A new templ helper `@copyButton(targetID)` renders a small button next to a
copy target. The button uses a Datastar `data-on-click` handler that calls
`navigator.clipboard.writeText` on the target's text content and flips an
icon class for 1.5s to flash a "copied" state.

Used on:
- The plaintext-token banner (after token mint)
- Every `.empty-state-snippet` block (already a single CSS class — apply once)
- The CLI install card's snippets (§1.2)

CSS: small icon-only button, accent on hover, success-color flash on copy.

---

## 3. P2a — `Cmd+K` global search

A single keyboard-triggered command palette scoped to the current team.

### 3.1 Endpoint

```
GET /t/:team/search?q=<query>
→ {"products": [...], "impls": [...], "features": [...], "branches": [...]}
```

Each result has `{label, href}`. Server-side: case-insensitive `LIKE` queries
across each table, scoped by team_id, capped at 5 results per group. Total
~20 results max. No FTS — punt to a future sprint when ACID-text search is in
scope.

### 3.2 Palette UI

A new templ component rendered once in `Shell` (so it's available on every
authed page):

- Modal overlay, hidden by default, controlled by Datastar signal `paletteOpen`.
- Toggled by `Cmd+K` (Mac) / `Ctrl+K` (others) via a Datastar key-handler on
  the body.
- Input field with autofocus when opened. `data-on-input__debounce.150ms`
  fires a `@get` to the search endpoint and merges the result fragments
  into the palette body.
- ↑/↓ moves selection (CSS class `is-selected` on the highlighted row), Enter
  navigates, Esc closes.

### 3.3 Datastar usage

Datastar v1 has `@get` for server-driven HTML fragment updates and key-event
modifiers — no custom JS needed. The palette body is a streamed fragment.

---

## 4. P2b — activity feed (per-page recents)

### 4.1 Schema

A new `events` table tracks first-class events:

```sql
CREATE TABLE events (
  id            TEXT PRIMARY KEY,
  team_id       TEXT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
  product_id    TEXT REFERENCES products (id) ON DELETE CASCADE,
  impl_id       TEXT REFERENCES implementations (id) ON DELETE CASCADE,
  feature_name  TEXT,
  actor_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
  kind          TEXT NOT NULL,
  payload       TEXT NOT NULL,
  inserted_at   TEXT NOT NULL
);
CREATE INDEX events_team_idx
  ON events (team_id, inserted_at DESC);
CREATE INDEX events_product_idx
  ON events (product_id, inserted_at DESC) WHERE product_id IS NOT NULL;
CREATE INDEX events_impl_idx
  ON events (impl_id, inserted_at DESC) WHERE impl_id IS NOT NULL;
```

`kind` enum (string-typed):
- `push.spec` — payload: `{branch, commit, feature_name}`
- `push.refs` — payload: `{branch, commit, feature_name, ref_count}`
- `status.changed` — payload: `{acid, from, to}`
- `token.minted` — payload: `{token_name, prefix}`
- `member.added` — payload: `{user_id, role}`
- `product.created` — payload: `{product_name}`
- `impl.created` — payload: `{impl_name, parent_impl_id}`

`actor_user_id` is the authenticated session user when available, NULL for
system-originated events (none in v1, but the column allows for it).

### 4.2 Write sites

Single-call `events.Emit(ctx, EmitParams{...})` in:
- `services.Push.Apply` — one event per affected feature, kind based on whether
  the feature carried a spec or refs (or both — emit two)
- The new inline-status-edit handler (§2.2) — one `status.changed` per write
- `Teams.CreateAccessToken` — `token.minted`
- `Teams.AddMember` — `member.added`
- `Products.Create` — `product.created`
- `Implementations.Create` — `impl.created`

Each emit is best-effort: log on failure, never block the request. Errors are
surfaced via slog.

### 4.3 Read surface

A single repo function:

```go
func (r *Repository) RecentForScope(ctx context.Context, scope EventScope, limit int) ([]*Event, error)
```

Where `EventScope` is `{TeamID, ProductID, ImplID}` with optional pointers.
The query picks the most-specific non-nil filter (impl > product > team) and
orders by `inserted_at DESC LIMIT ?`.

A new templ component `@recentActivity(events []*Event)` renders a quiet
strip:
- 5-row max; each row is one line of text.
- Format: `<actor> <verb> <target> · <relative-time>` — e.g.
  `jadams pushed 3 specs to auth · 2m ago`.
- `<verb>` and `<target>` are derived from `kind` + `payload`.
- The whole strip is a single `<section>` with the same `.panel` styling as
  other sections, slightly less prominent.

Mounted on:
- Team page — above the heatmap, scope = `{TeamID}`
- Product page — above the progress banner, scope = `{ProductID}`
- Impl page — above the tracked-branches table, scope = `{ImplID}`

If the scope has zero events, the strip does not render.

---

## 5. PR sequence

| # | Branch | Lands |
|---|---|---|
| 1 | `feat/ux-onboarding-magic-link` | §1.1 magic-link confirmation page |
| 2 | `feat/ux-cli-install-card` | §1.2 + §1.3 sequenced empty states + token-mint next-step block |
| 3 | `feat/ux-copy-buttons` | §2.3 — copy button helper, applied across all snippet blocks |
| 4 | `feat/ux-index-progress` | §2.1 — progress bars on impls, features, branches indexes |
| 5 | `feat/ux-inline-status-edit` | §2.2 — `<select>` + new POST route + tests |
| 6 | `feat/ux-cmdk-palette` | §3 — search endpoint + palette overlay |
| 7 | `feat/ux-activity-events` | §4 — events table + emit sites + per-page recents strip |

Each is independently shippable. Order chosen so onboarding fixes land first
(unblocking new users), then page-level UX lifts that cost almost nothing,
then the structurally larger items (palette + events).

---

## 6. Acceptance criteria per PR

Each PR must:
- Pass `just precommit` (gofmt, go fix, vet, golangci-lint v2, race tests)
- Add at least one render or handler test for the new behavior
- Update existing tests rather than adding new test files when the change
  modifies an existing handler
- Ship behind no feature flag — these are user-visible and per-PR safe

---

## 7. Risks and unknowns

- **§3 search palette size**: 4 LIKE queries × per-keystroke (debounced 150ms)
  is fine at current data scales, but a team with 500 specs would feel it. The
  result cap (5 per group) bounds the response size; the query cost is the
  open question. Mitigation: index the `name` columns we're searching (most
  already are; verify in implementation).
- **§4 events write-cost**: the worst case is a `acai push --all` that
  touches 50 features — that's 50+ event rows in one transaction. The events
  index is on `(team_id, inserted_at)` so write cost should be ~constant per
  row. Mitigation: emit events in the same transaction as the underlying
  domain write, batch-insertable if it ever bites.
- **§4 schema migration**: adding a new table + indexes + sqlc queries is
  routine, but the `events` table name is generic and may collide with future
  ideas. Naming chosen to be intentionally generic — this IS the events table.
- **§2.2 authorization**: any team member can edit status. Per-impl ownership
  / per-status authorization is deferred. Acceptable for current usage; flag
  in the PR description so reviewers see it.

---

## 8. Tracking

After this design is approved, the writing-plans skill produces a step-by-step
implementation plan for PR #1 first; subsequent PRs get their own plans as
each prior one merges.
