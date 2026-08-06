# CLAUDE.md

**`AGENT.md` is the source of truth for vision, principles, and package
boundaries.** Read it before any non-trivial change. This file distills it, adds
the commands it does not list, and records the service/stack decisions made
after it was written (see "Service map" and "Stack decisions" — where those two
disagree with AGENT.md, they are newer).

The code on disk is a skeleton — an empty `main.go`, `doc.go` stubs, and
untouched create-next-app output. Do not infer patterns, conventions, or scope
from it. AGENT.md describes what this is meant to become; the files describe
nothing yet.

## What Switchyard is

An AI-powered workflow automation platform for software teams. The user
describes a workflow in natural language, AI generates an **editable visual
graph**, and the user inspects and edits it before anything executes.

Built for engineers, DevOps, and platform teams — deliberately *not* a
general-purpose Zapier. Marketing, HR, and sales automation are out of scope.

## Principles that constrain every change

- **AI assists, never owns.** Every AI-generated action must be editable by the
  user. AI accelerates workflow creation; it does not replace developer control.
- **Explainability.** Execution is never a black box. What happened, why, what
  data was used, which AI response came back, which node failed — all visible.
- **Deterministic execution.** A saved workflow executes the same way until the
  user intentionally changes it.
- **Developer first.** The UI must never make a developer feel constrained.
- **Simplicity.** Introduce infrastructure only when there is a real need.
- Prefer boring, explicit, readable code. Small interfaces, composition over
  inheritance, clear package boundaries. No clever code.

## Architecture

Next.js frontend → REST + WebSocket → Go API server (Chi) → services → workflow
engine → PostgreSQL and external APIs. **Modular monolith** — the "services"
below are packages in one binary, not deployables. Microservices are not a goal.

Frontend surfaces: dashboard, workflow builder (React Flow canvas), execution
viewer, AI prompt UI, settings.

The Go API server owns auth middleware, REST routing, the WebSocket gateway,
request validation, and authorization — nothing else.

### Service map

Called by the API layer:

- **Auth** — verify JWT, build user context, sessions, permissions.
- **Workflow** — CRUD, graph validation, versioning, templates.
- **Execution** — start, retry, cancel, schedule.

Below execution:

- **Workflow engine** — graph traversal, node execution, variables, branching,
  error handling. The heart of the platform; knows nothing about HTTP.
- **Credential service** — encrypt/decrypt/rotate provider keys, OAuth tokens.
  Secrets never leave it in plaintext and never land in logs or execution
  output.
- **AI service** — prompt building, provider selection, model selection,
  response handling.
- **Integration service** — GitHub, Slack, Discord, generic HTTP.
- **Notification service** — WebSocket fan-out and events (email is future).
- **Execution logs** — live logs, audit trail, metrics.

PostgreSQL tables: users, workflows, versions, executions, logs, credentials,
templates.

### Stack decisions

| Layer     | Choice                | Note                                     |
| --------- | --------------------- | ---------------------------------------- |
| Frontend  | Next.js + React       | Full breakdown below                     |
| Auth      | **Better Auth**       | Runs in Next.js; Go **verifies** the JWT  |
| Backend   | Go + Chi              |                                          |
| Database  | PostgreSQL            | JSON columns for graphs                  |
| Realtime  | WebSockets            | Live execution updates                   |
| AI        | Provider abstraction  | Default **OpenRouter**; native OpenAI, Anthropic, and Gemini adapters share the same interface, Azure later |
| Storage   | Local now, S3 later   | Execution artifacts and uploads          |

### Frontend stack

| Layer           | Technology            | Why                                                                         |
| --------------- | --------------------- | --------------------------------------------------------------------------- |
| Framework       | Next.js (App Router)  | Modern React framework with Server Components and routing                   |
| Language        | TypeScript            | Type safety across the frontend                                             |
| Styling         | Tailwind CSS          | Utility-first styling                                                       |
| UI Components   | shadcn/ui             | Accessible, customizable component library                                  |
| Icons           | Lucide React          | Consistent icon set                                                         |
| Workflow Canvas | React Flow            | Visual node-based workflow editor                                           |
| HTTP Client     | **axios**             | One instance in `lib/api.ts`; an interceptor mints the JWT per request      |
| Server State    | **TanStack Query**    | API fetching, caching, mutations, optimistic updates, background refetching |
| Client State    | Zustand               | UI state like selected node, sidebar, canvas preferences, dialogs           |
| Forms           | React Hook Form + Zod | Performant forms with schema validation                                     |
| Authentication  | Better Auth           | Authentication with JWT verification for the Go backend                     |
| Realtime        | Native WebSockets     | Live workflow execution and logs                                            |

**The state split is the rule to follow**: anything that came from the API
belongs to TanStack Query, and anything that is purely UI belongs to Zustand.
Copying server data into a Zustand store is how the two drift apart.

Of these, only Next.js, TypeScript, Tailwind, and Better Auth are installed.
The rest arrive with the features that need them — do not add them in advance.

Two consequences worth remembering: Better Auth means signup, login, and
password hashing live in the frontend, so `internal/auth` is JWT verification
and session/permission lookup only. And OpenRouter being the default never
justifies calling it directly — it is one implementation behind the AI provider
interface, same as the rest.

### Auth wiring, as built

- `frontend/src/lib/auth.ts` — the Better Auth server instance: pg pool,
  email/password, and the `jwt` plugin issuing 15-minute tokens with issuer
  `BETTER_AUTH_URL` and audience `switchyard-backend`.
- `frontend/src/app/api/auth/[...all]/route.ts` — mounts the handler.
- `frontend/src/lib/auth-client.ts` — React client plus `getToken()`, which the
  frontend calls per request to the Go API.
- `backend/migrations/0001_better_auth.sql` — generated tables: `user`,
  `session`, `account`, `verification`, `jwks`. Regenerate with `pnpm
  gen:schema`, do not hand-edit.

- `backend/internal/auth/jwt.go` — `Verifier.Verify` fetches the JWKS from
  `/api/auth/jwks`, caches it, and checks signature, issuer, audience, and
  expiry. **Ed25519 only, by deliberate choice**: a token naming any other alg
  is rejected before a key is selected, so algorithm-confusion attacks have
  nowhere to land. That is why the backend needs no JWT dependency —
  `crypto/ed25519` is stdlib. Do not "generalize" it to accept more algorithms.

- `backend/internal/api/api.go` — `RequireAuth` middleware: pulls the bearer
  token, verifies it, and puts the claims on the request context via
  `auth.NewContext`. Handlers read them with `auth.FromContext`. A 401 says only
  "invalid token" — which check failed is useful to someone probing tokens and
  useless to an honest client, whose only move either way is to get a new one.
- `backend/internal/config/config.go` — the only reader of env vars:
  `SWITCHYARD_ADDR`, `SWITCHYARD_AUTH_ISSUER`, `SWITCHYARD_AUTH_AUDIENCE`,
  `SWITCHYARD_ENV`, `SWITCHYARD_LOG_LEVEL`. A
  non-absolute issuer is a startup error, since it would otherwise surface as
  every token looking invalid.

`scripts/verify-auth.mjs` performs the same check in JS against a live server;
`jwt_test.go` covers the Go side, including a captured real token as a golden
case so a Better Auth format change fails a test rather than production.

**Routing is Chi** (`github.com/go-chi/chi/v5`, the backend's only dependency),
chosen for readability. The canonical Go API is versioned under `/api/v1`; the
original `/api` paths remain a compatibility alias while clients migrate.
Better Auth's frontend endpoints under `/api/auth` are separate and are not
part of this versioned Go API. Protected routes go inside either API group,
which applies `RequireAuth` to the whole subtree:

    router.Route("/api/v1", func(r chi.Router) {
        r.Use(RequireAuth(verifier))
        r.Get("/me", handleMe)
    })

Signed GitHub webhooks use `/api/v1/hooks/github/{workspaceID}/{workflowID}`.
The original `/hooks/github/{workspaceID}/{workflowID}` path remains available
for installations that already configured it; webhook requests authenticate
with GitHub's signature instead of a user JWT.

One consequence: an unknown path under `/api/v1` (or its `/api` compatibility
alias) answers **401, not 404**, because group middleware runs before the
subtree resolves a route. That is the better default — it stops an
unauthenticated caller enumerating which endpoints exist. With a valid token the
same path correctly 404s. A route that must be public
belongs outside the group, like `/healthz`.

`middleware.Recoverer` is mounted so a panic fails one request instead of the
process.

### Permissions

**Everything belongs to a workspace, not a user.** Workflows, executions, and
credentials are workspace-owned, so access is a membership question. Modelled
on pulseops, which solved the same problem.

`internal/auth` holds the role model — four strictly ordered roles and one
permission table in `role.go`. `internal/workspace` holds membership,
invitations, and the rules about who may change whom.

    VIEWER  workflow:read, execution:read, member:read
    MEMBER  + workflow:write/delete, execution:run
    ADMIN   + credential:manage, member:manage, workspace:update
    OWNER   + workspace:delete

**`minimumRole` in `role.go` is the whole authorization table.** Add a
permission there or it is denied to everyone — an unknown permission returns
false rather than falling through to the zero role, so a typo at a call site
closes a door instead of opening one. An unrecognised *role* ranks below
VIEWER for the same reason.

Three rules in `workspace` exist to stop privilege escalation, each with a
test:

- a member may not grant a role above their own (an admin cannot mint an owner)
- a member may not act on a peer (two admins cannot demote each other)
- the last owner may not be demoted or removed

`api.RequirePermission(svc, perm)` gates a route on the `{workspaceID}` URL
parameter. **It checks per request, not off the token**, so a demotion applies
immediately rather than when the 15-minute JWT expires. Mounted on a route with
no `{workspaceID}`, it fails closed.

`writeError` owns the status mapping:

    auth.ErrNoIdentity        -> 401
    workspace.ErrNotMember    -> 404   (not 403)
    workspace.ErrNotFound     -> 404
    workspace.ErrForbidden    -> 403
    ErrInviteExpired/Exhausted-> 410
    workspace.ErrLastOwner    -> 409
    anything else             -> 500, cause logged, generic body

**Non-membership answers 404 while an insufficient role answers 403**, and the
split is deliberate. A stranger must not learn a workspace id is real, so their
answer is identical to one for a workspace that never existed — there is a test
comparing the two bodies. A member already knows it exists, so 403 tells them
nothing they could not see anyway.

**Only the SHA-256 of an invite token is stored.** The token is a bearer
credential; storing it would let anyone reading the database join every
workspace with an invite outstanding. It is returned exactly once, at creation
— to re-share a link you revoke and re-issue. Accepting an invite twice is not
an error, and an existing member keeps the role they have, so a viewer link
cannot be used to demote an admin.

`auth.UserID(ctx)` is still the caller's id and still the value owner-scoped
queries filter by. `auth.RequireOwner` remains for rows owned by a user
directly rather than a workspace.

### Database

**pgx v5** with `pgxpool`, opened in `main.go` and passed down. `database.Pool`
is a type alias for `*pgxpool.Pool` rather than a wrapper — wrapping would mean
re-exporting every method a caller needs, one at a time, forever.

`DATABASE_URL` is **required** to boot, and uses that name rather than a
`SWITCHYARD_` prefix because the frontend, `psql`, and every Postgres tool
already read it; two spellings of one connection string is how they drift.
`SWITCHYARD_DB_MAX_CONNS` bounds the pool.

`database.Connect` pings before returning. pgxpool connects lazily, so without
that a bad URL would look like a healthy start and fail on the first request.

**Migrations run at startup**, from `backend/migrations` embedded via
`migrations.FS`:

- ordered by the **number** in the filename, not lexically, so 10 comes after 9
- each migration and the row recording it share one transaction, so a failure
  leaves neither a half-applied schema nor a false record — Postgres has
  transactional DDL, which is what makes that work
- a Postgres advisory lock serialises instances starting together
- `Verify` refuses to start a binary older than the schema it is pointed at,
  which is the usual way a rollback goes wrong quietly

Add a migration as `NNNN_name.sql` and nothing else — no registration step. Two
files sharing a version, an unnumbered name, or an empty file all fail at test
time rather than at boot.

**Queries do not live here.** `database` owns the pool and the schema; each
domain package declares its own `Store` interface and its own SQL. Putting
every query in one package is how it becomes the package everything imports and
nothing can be tested without.

Integration tests need a database and skip without one. They drop and recreate
the `public` schema, so **`database.CheckTestURL` refuses to run when
`SWITCHYARD_TEST_DATABASE_URL` resolves to the same database as
`DATABASE_URL`** — same host and database name, comparing where the URLs land
rather than how they are spelled, so `localhost` and `127.0.0.1` and a stray
`?sslmode=` do not slip past. That guard exists because the two strings are
similar enough to paste one where the other belongs, and doing so once cost
this project its development database. Point them at a throwaway:

    SWITCHYARD_TEST_DATABASE_URL=postgres://postgres:t@localhost:55433/switchyard \
      go test ./internal/database/

### CORS

`CORS(appURL)` allows exactly one origin — the frontend's URL, which is already
the JWT issuer, so there is no second value to keep in step. Not `*`: that
would let any page on the internet drive this API with a token it obtained.

It is mounted **before `RequireAuth`**, because a preflight carries no
`Authorization` header. Behind auth it would get a 401, and the browser would
never send the real request — a failure that looks like broken CORS but is
actually middleware ordering.

### Logging

**zerolog**, built in `main.go` and passed explicitly into `api.NewRouter`.
Chi's own `Logger` is not mounted — `RequestLogger` in `api.go` replaces it and
emits structured fields instead of formatted text.

    SWITCHYARD_ENV=development   # console output; "production" switches to JSON
    SWITCHYARD_LOG_LEVEL=info    # trace|debug|info|warn|error|fatal|panic

One line per request, after it completes, carrying method, path, status, bytes,
duration, and the Chi request ID. `/healthz` logs at **debug** — a load balancer
polls it every few seconds and it would otherwise drown everything else. A 5xx
logs at error.

**Rejected tokens are logged at warn with the real reason**, while the response
still says only "invalid token". Operators need to tell a misconfigured issuer
from an actual attack; the client does not get that detail. Keep that split when
adding auth failure paths.

**`BETTER_AUTH_SECRET` encrypts the JWKS private key stored in the `jwks`
table.** Changing the secret without clearing that table makes every token mint
fail with `Failed to decrypt private key` — while `/api/auth/jwks` keeps
serving the stale public key, so the symptom is a 500 on `/token`, not on the
key set. Fix: `truncate jwks`, or put the old secret back.

**Ports on this machine are all taken by other projects.** Postgres is 5434
because 5432 and 5433 belong to pulseops and auramail; the API is **8090**
because auramail's backend container holds 8080; the frontend is 3007 because
3000 and 3001 are in use. All three are defaults in code, not just docs, so a
bare `go run` and `pnpm dev` land somewhere free.

**The frontend is pinned to port 3007** via `--port` in `package.json`, because
3000 and 3001 belong to other projects on this machine and Next silently drifts
to a free port when its default is taken. `BETTER_AUTH_URL` becomes the token
issuer and the Go verifier compares it exactly, so the port and that variable
have to move together — changing one alone breaks verification quietly.

To check both sides agree against a running server:

    SWITCHYARD_LIVE_ISSUER=http://localhost:3007 \
    SWITCHYARD_LIVE_TOKEN=$(curl -s -b cookies.txt http://localhost:3007/api/auth/token | jq -r .token) \
    go test ./internal/auth/ -run TestVerifyAgainstLiveIssuer -v

That test skips unless both variables are set.

## Rules for backend code

Layout and responsibilities are spelled out in AGENT.md's "Folder Philosophy";
the rules that matter most when writing code:

- Business logic lives in `internal/` packages, never inside HTTP handlers.
  `cmd/switchyard` does wiring and startup only.
- Dependency direction is one-way:
  `api → auth, workflow, execution`; `execution → ai, github, slack, database,
  websocket`; everything → `database, config`. Domain packages never import
  `api`. The engine never imports HTTP. **A cycle means a boundary was drawn
  wrong.**
- Every package carries a `doc.go` stating its responsibility. If that takes
  more than two sentences, the package is doing too much.
- The transport package is `api`, never `http`, so it cannot shadow `net/http`.
- `config` is the only package that reads env vars directly.
- `pkg/` stays empty until something genuinely general-purpose earns a place.
- A workflow is **data**; it does not run anything. Running belongs to
  `execution`. The engine is designed independently of the UI: the frontend
  visualizes, the backend executes.
- Services from the map above land in the `internal/` package that already
  claims them — integrations in `github`/`slack`, notifications in `websocket`,
  logs in `execution`. Key material stays in `internal/credential` rather than
  being smeared across the packages that use it.

### Credentials

**Scoped to a workspace, not a user.** A credential belongs to the workspace,
so a key one member saves is the key everyone's workflows run with, and
`credential:manage` at ADMIN decides which keys exist. `0002` originally scoped
them to a user; `0004` moved them.

`credential` scopes but does not authorize — it takes a workspace id and
trusts it. Whether the caller may touch that workspace is settled earlier, by
`api.RequirePermission(svc, auth.PermissionCredentialManage)`. Keeping the
check in the transport layer means one place decides, rather than every
package inventing its own opinion.

The workspace id is also baked into the GCM additional data
(`workspace\0provider\0name`), so it is a second lock rather than only a query
predicate: a ciphertext moved between workspaces fails to open. `Service.Get`
builds that binding from the workspace it was *asked* for, so a store that ever
dropped its `WHERE` clause returns a decryption failure instead of another
workspace's key.

### Workflows

**A workflow is data.** `internal/workflow` owns the graph, its validation, and
its rows; it runs nothing. `0005` adds the `workflow` table.

**The graph is one `jsonb` column, not normalized node and edge tables.**
Nothing ever reads a single node: the builder sends the whole graph, the engine
walks the whole graph, a diff compares whole graphs. Splitting it would cost a
join and N inserts per autosave and buy back a query nobody makes.

**The graph uses React Flow's field names**, on purpose: `source`/`target`/
`sourceHandle` on an edge, `data` on a node. The frontend holds exactly this
shape in `useNodesState`/`useEdgesState`, so a save is the array the canvas
already has. There is no mapping layer, and there should not be — a translation
is somewhere for the two representations to drift apart. A live round trip is
byte-identical.

**Validation is split in two, and the split is the important part.**

    Graph.Validate()   save time   the document is intact
    Graph.Runnable()   run time    the graph can actually execute

`Validate` checks unique non-empty ids, that every edge endpoint is a real node,
size caps, and that `data` is valid JSON. That is all. **It does not ask whether
the workflow could run**, because a builder canvas is half-finished by
definition: a node dropped but not yet wired, no trigger yet, two triggers
mid-edit. Autosave fires constantly during that, and rejecting it would mean
saves failing while somebody is mid-thought. **A save is a draft.**

`Runnable` is the set of guarantees the engine may assume — exactly one trigger,
nothing pointing into it, no cycles, every node reachable, a known node-type
category. The execution service calls it; saving never does. A graph that fails
it is a perfectly good draft.

This was originally one save-time gate, and it was wrong: `TestBuilderDraftsSave`
is the test that would have caught it.

**Only the category is checked, not the action** — `http.` is known,
`http.request` is not verified. Whether an action exists is the node registry's
question and the registry ships with the engine. That check sits in `Runnable`
so the frontend can ship a node type before Go learns about it, costing a failed
run rather than a failed save. `categories` is the one place to add a family.

**Cycles are rejected** so the engine can be a topological walk rather than a
loop detector carrying a step budget. Looping, if it is ever wanted, is an
explicit node type with a visible bound.

`Node.Data` is `json.RawMessage` — the label the canvas draws plus whatever the
node type needs — and is only checked for being well-formed JSON. This package
does not know what an AI node requires, and keeping it opaque means an
unrecognised field survives a save/load round trip.

**No `workflow_version` table, deliberately.** Determinism is what would demand
one, and pinning the graph on the execution row satisfies that completely —
an execution keeps what it ran, so a later edit cannot change what it appears
to have done. History and rollback are a product feature and an additive
migration when they are wanted.

**Update is read-modify-write with no locking**: two people editing one workflow
means the later save wins. The fix, when that stops being acceptable, is a
revision column the client echoes back, not a transaction.

`Patch` fields are pointers, so "not sent" and "set to empty" stay different
things — the builder autosaves the graph alone and that must not blank the
description.

Every `Store` method takes a workspace id and puts it in the `WHERE` clause. A
workflow id alone would find the row, which is the danger: `RequirePermission`
checked the workspace in the URL, so a lookup by id alone hands workspace B's
admin a workflow from workspace A. `TestStoreContract` runs one set of
expectations against `MemoryStore` and `PostgresStore` so the two cannot drift
the way the workspace slug rule once did.

Routes sit under `/workspaces/{workspaceID}/workflows`, gated on
`workflow:read` (VIEWER), `workflow:write`, and `workflow:delete` (MEMBER).
`writeError` maps `workflow.ErrInvalid` and `ErrNotRunnable` to **400 carrying
the reason** — the caller wrote the graph, so `edge "e1" ends at unknown node
"ghost"` is theirs to know and describes only what they just sent. That is the
opposite of the auth failures, where the reason goes to the log and never the
response.

Graph bodies use `decodeJSONLimit` at 1 MiB; the default 64 KiB is for the
endpoints that take a handful of short fields.

`frontend/src/lib/api.ts` is the other half: one axios instance whose request
interceptor calls `getToken()` and sets the bearer header. **That interceptor is
why axios is there at all** — a Better Auth token lives 15 minutes, so it has to
be minted per request, and doing that by hand at each call site is one forgotten
`await` away from a 401 that looks like a permissions bug. `apiError(err)` pulls
the backend's `{"error": ...}` out of a failure, since axios's own `err.message`
would replace "duplicate node id" with "Request failed with status code 400".

### Executions

**`internal/execution` is the engine.** It walks a graph, runs each node,
records what happened, and knows nothing about HTTP. `0006` adds `execution`
and `execution_node`; `0008` adds retry lineage and durable idempotency keys.

**Starting is asynchronous.** A workflow calls external services and can take
minutes, so `POST .../executions` answers **202** with an id and the caller
watches it. The run gets `context.WithoutCancel` — the request's context dies
when the response is written, and using it would kill every run the instant it
was accepted — plus a `runTimeout` of its own.

**The graph is copied onto the execution row.** That snapshot is what makes a
finished run mean something: editing the workflow afterwards cannot change what
this run appears to have done. **It is also why there is no `workflow_version`
table** — the snapshot answers the question versioning would have been asked.

**`Runnable` is checked at `Start`, not at save.** This is where the draft/run
split from `workflow` pays off: a half-built canvas saves all day, and starting
it is the moment it has to be a real workflow.

Nodes run **one at a time**, in `topological` order seeded in graph order rather
than map order — deterministic execution is the promise, and a map range would
quietly break it. Parallel branches are an optimisation to make when something
is visibly waiting on it.

**Branching is `sourceHandle`.** After a node runs, an edge is followed when its
handle is empty (the default output) or matches the `Branch` the node returned.
A node nothing activates is recorded **SKIPPED**, which is a real outcome: an
untaken branch has to look different from a step still pending.

**Variables are `text/template` over the node's `data`**, with
`.nodes.<id>.<field>` and `.trigger.<field>`. Stdlib rather than an expression
language of our own — one less parser to write, document, and get wrong.
`missingkey=error` is set on purpose: a workflow that silently posts `""` where
a PR number belongs is worse than one that stops and says which reference was
wrong. A value that may contain quotes needs `{{ json . }}`, or the substitution
breaks the surrounding JSON and is rejected with a message saying so.

**A failed node keeps its output.** An HTTP node rejected with a 401 still holds
the body explaining why, and that is the whole reason anyone opens a failed run.
`dispatch` returns the result *alongside* the error; dropping it there was a real
bug, caught live rather than by a test, and `TestFailedNodeKeepsItsOutput` now
pins it.

**Runners live with their integration.** `Registry` maps a node type to a
`Runner`; GitHub nodes belong in `internal/github` and so on. `Builtin` holds
only the ones needing nothing but stdlib — triggers, `logic.condition`,
`logic.switch`, `logic.delay`, `variable.set`, and `http.request` — because a
package containing one function is ceremony, not a boundary.

**`variable.set` computes nothing.** The template layer has already substituted
its data; it only decides what the node's output is. Its `values` object
*becomes* the output, so a reference is `.nodes.<id>.<name>` — the same shape as
every other node, not a wrapper only this type has. `values` must be an object:
a list or a scalar would fail later inside a template, naming the node that
referred to it rather than the node that is wrong. **An unregistered node type fails its run** with a message naming the
type; silently doing nothing is the one outcome an engine must never have.

**`Reclaim` runs at startup, before the server listens.** A process that dies
mid-run leaves rows nothing will ever finish, and a run stuck at RUNNING reads
as "the engine is wedged" rather than "this never completed". `Finish` will not
overwrite a status that is already terminal, so a node completing just as a
cancel lands cannot turn CANCELLED into SUCCEEDED — both stores are tested on
that.

**Cancel only reaches runs in this process.** That is honest rather than
limiting: one binary is the whole deployment. A cancel for a run this process
has never heard of finishes the row instead.

**Retry is explicit recovery, not automatic node replay.** Only FAILED and
CANCELLED runs can be retried, and the new run carries the original graph
snapshot, input, and a `retryOf` link. Automatic retries stay out of the engine
because repeating an external side effect without a node-level idempotency
contract can make the failure worse.

**Idempotency keys are durable and workspace-scoped.** Start and retry accept
`Idempotency-Key`; the same key and request return the original run, while a
different request gets a conflict. GitHub delivery ids and scheduler slots use
the same mechanism, so redelivery or a restart cannot duplicate a run.

Routes: `POST .../workflows/{workflowID}/executions` and
`POST .../executions/{id}/cancel` and `POST .../executions/{id}/retry` need
`execution:run` (MEMBER); listing and reading need `execution:read` (VIEWER) —
a viewer may watch what happened, and it takes a member to make something
happen.

**`execution` imports `workflow`.** That arrow is not in AGENT.md's list and is
correct: a workflow is data and the engine consumes it. `workflow` imports
nothing back, so there is no cycle.

### AI

**`internal/ai` is the only package that talks to a model.** Two jobs:
generating a workflow graph from a description, and running the AI node types
(`ai.prompt`, `ai.chat`, `ai.summarize`, `ai.classification`, and
`ai.decision`). No migration — the key is an ordinary credential.

**Generating stores nothing.** `POST .../workflows/generate` returns
`{name, description, graph}` and creates no row. The canvas opens with it and
the user saves it through the ordinary create endpoint once they have looked at
it. That is "AI assists, never owns" in one route: a generate-and-save would
put a workflow in the list nobody has read. It sits at `workflow:write`
anyway — it spends the workspace's model budget.

**The key is a per-workspace credential** (`<provider>`/`default`), fetched per
call, never cached and never held on a struct. Supported providers are
`openrouter` (the default), `openai`, `anthropic`, and `gemini`. `Provider.Complete`
takes the key as a *parameter* rather than a field for that reason: one
process-wide provider serves every workspace and no long-lived value holds a
secret. A workspace with no key gets `ErrNoCredential` → **400 naming the fix**,
not the 502 an upstream failure gets.

**The generated graph is checked with `Validate`, not `Runnable`.** It is a
draft heading for a canvas, so the same half-finished states a person may save
are allowed here; what it must not be is malformed, which would break the
editor rather than the run.

**One retry, carrying the reason.** A model that returns an unusable graph is
told what was wrong and asked again, once. Two failures is `ErrBadGraph` →
**502**: the call succeeded and the content was the problem, so the caller's
only move is to try again. `stripFence` unwraps a ```` ```json ```` block,
since a model that wrapped a correct answer has still answered.

**`systemPrompt` lists only node types that have a runner.** Advertising one
the engine cannot execute buys a workflow that saves and then fails on its
first run. Update it when a runner package lands. `layout` spaces nodes out
when the model gave no positions, or the canvas opens with everything stacked
at 0,0.

**`ai` imports `execution`, never the reverse.** `ai.Runners(service)` returns
a `Registry` that `main.go` merges in — the same shape `github` and `slack`
will use. The engine knows the `Runner` interface and nothing about models.

**An empty graph is rejected here even though `Validate` allows one.** An empty
canvas is a legitimate thing for a *person* to save and never a legitimate
answer from a model — `{"name": "deploy on merge"}` and nothing else would
otherwise be a 200 opening a blank canvas, which reads as our bug rather than
the model's.

**The deadline lives on the caller's context, not on the HTTP client.** The two
callers are not the same: generation answers an HTTP request and has to fit
under the server's 30s write timeout (`generateTimeout`, 25s, covering both
attempts), while an AI node has the engine's per-node budget and
nobody waiting on a connection. A client timeout tuned for the first silently
cut the second short — two mechanisms bounding one call is how that happens
unnoticed. `openRouterBackstop` is 2 minutes and exists only so a caller that
forgets a deadline cannot hang forever. `DefaultModel` is one constant; a
workflow that pinned a model keeps it.

### Live execution events

**`internal/websocket` is a transport and holds nothing worth reading.** The
rows are the record; this is a notification. That is what lets a slow client be
dropped rather than waited for. `github.com/coder/websocket` is the second
direct dependency after chi — it has none of its own.

**Neither package imports the other.** `execution` declares a one-method
`Publisher` it announces progress to; `api` declares a one-method `EventStream`
it serves. `websocket.Hub` satisfies both, and `main.go` is the only place that
knows they are the same object.

**`Publish` never blocks and never fails.** The engine calls it while running a
workflow, so a browser somebody closed the lid on must not be able to hold a
run up. A client more than `sendBuffer` events behind is **dropped**, and
reconnects to state it re-reads over REST.

**Nothing is announced that was not written.** Events fire after the row lands
and only if it landed — announcing a status the database never took leaves the
watcher showing one thing while a refresh shows another, and later a third once
`Reclaim` catches the row. Silence is the honest outcome: the client keeps
showing what the database agrees with. `record` covers every node transition
(RUNNING, its outcome, and SKIPPED), so one call site does the lot. Output over
64 KiB is dropped from the event with `outputTruncated`: a megabyte written once
to a row is a different budget from a megabyte buffered per watcher.

**`Cancel` announces its own outcome when no engine will.** Cancelling a run
this process is not running finishes the row directly, so nothing else is going
to tell a watcher — they would sit on RUNNING for a run the database has
cancelled.

**`Serve` subscribes before the handshake, not after.** `Accept` writes the 101
that releases the client, so subscribing afterwards races it — the client
believes it is connected while the hub has never heard of it. Events published
in that window go nowhere, which is the exact gap the ordering below exists to
close. `TestSubscribedBeforeTheHandshakeCompletes` fails under `-race -count 200`
if it is ever moved back.

**Connect before you fetch. This is the contract.** Nothing is replayed.
Connect first and the fetch returns either a finished run or a running one
whose remaining events all arrive; fetch first and a run that ends in the gap is
a page showing RUNNING with nothing left to correct it. A replay buffer would
remove the ordering requirement and add a second copy of state that can
disagree with the rows.

**The token rides in `Sec-WebSocket-Protocol`, not a query parameter.** The
browser's WebSocket constructor cannot set headers, and a URL is the one place
a bearer token must not go — URLs are what access logs, proxies, and Referer
headers record. `bearerToken` falls back to `bearer, <token>` in that header, so
there is one authentication path rather than a second one for WebSockets to get
wrong.

**The handler looks the execution up before subscribing, and that lookup is the
authorization.** `RequirePermission` checks the `{workspaceID}` in the URL and
knows nothing about the `{executionID}` beside it, so without it read access to
one workspace would be read access to every run on the server.

`OriginPatterns` comes from `appURL` for the same reason CORS does — the
frontend is a different origin, and `Accept` rejects cross-origin by default.
Route: `GET .../executions/{executionID}/events`, gated on `execution:read`.

Two things worth knowing when testing: `net/http` clears a connection's
deadlines on hijack, so a stream outlives the server's `WriteTimeout` — the
hub's tests set a real one, because on a server without it that test passes
while proving nothing. And a client that stops calling `Read` still has a kernel
receive buffer, so the drop path cannot be reached end to end with small
messages; it is tested against the hub directly.

### The router takes a struct

`api.NewRouter(api.Deps{...})`, not a parameter list. Five same-typed service
pointers in a row can be swapped without the compiler noticing, and the tests
that passed `nil, nil, nil, testAppURL` were the warning. A new service is a
new field, and every existing call site keeps compiling.

## Commands

Postgres, from the repo root:

    docker compose up -d      # switchyard-postgres on 127.0.0.1:5434
    docker compose down       # add -v to drop the volume and reseed on next up

Port 5434, not 5432 — other projects on this machine hold the lower ports.

**The server applies migrations itself at startup**; compose no longer mounts
an init directory. That mount only ran on an empty volume, so a migration added
later never reached an existing database — which is how `0002_credentials` came
to be missing from the dev database while its file sat in the repo.

**The local dev database has already been fixed this way** — it was baselined
at 0001 and 0003, the server then applied the missing `0002_credentials`, and
its six users were kept. The rest of this note is for any other database seeded
by the old mount.

Such a database has the tables but no `schema_migrations`, so the runner tries
to re-apply `0001` and stops with `relation "user" already exists`. It fails
cleanly and changes nothing. Either `docker compose down -v`, or record what is
already there once:

    create table if not exists "schema_migrations" (
      "version" bigint primary key, "name" text not null,
      "appliedAt" timestamptz default CURRENT_TIMESTAMP not null);
    insert into "schema_migrations" ("version","name")
      values (1,'0001_better_auth.sql'), (3,'0003_workspaces.sql')
      on conflict do nothing;

The next start then applies only what is genuinely missing.

Backend (`backend/`, Go 1.26, module `github.com/R7rainz/switchyard/backend`):

`config.Load` reads `backend/.env` if it is there, so `go run` works in a bare
shell. **A variable already set in the environment always wins**, so
`DATABASE_URL=... go run ./cmd/switchyard` means what it says. `.env` and
`.env.example` are both gitignored, so a fresh clone has neither — these are
the variables to put in one:

    DATABASE_URL                # same database the frontend uses, port 5434
    SWITCHYARD_CREDENTIAL_KEY   # 1:$(openssl rand -base64 32), newest first
    SWITCHYARD_AUTH_ISSUER      # must equal the frontend's BETTER_AUTH_URL
    SWITCHYARD_AUTH_AUDIENCE    # switchyard-backend
    SWITCHYARD_ADDR             # :8090
    SWITCHYARD_ENV              # development | production
    SWITCHYARD_LOG_LEVEL        # trace|debug|info|warn|error|fatal|panic

`DATABASE_URL` and `SWITCHYARD_CREDENTIAL_KEY` are required; the server refuses
to start without them.


    go run ./cmd/switchyard     # :8090, override with SWITCHYARD_ADDR
    go build ./...
    go test ./...
    go vet ./...

Frontend (`frontend/`, **pnpm only** — never npm or yarn; pinned via the
`packageManager` field in `package.json`, which corepack enforces):

    pnpm dev
    pnpm build            # needs DATABASE_URL set: auth.ts fails fast without it
    pnpm lint
    pnpm gen:schema       # print Better Auth SQL; needs a reachable database
    pnpm verify:auth      # sign up -> mint JWT -> verify against JWKS

Copy `frontend/.env.example` to `.env` before running any of them.

`frontend/pnpm-workspace.yaml` gates dependency build scripts; a new dep whose
postinstall must run needs an `allowBuilds` entry.

## MVP scope

Auth (Better Auth signup/login, JWT verified in Go), dashboard CRUD over workflows, drag-and-drop
builder, AI workflow generation, execution with status/logs/timing, live log
streaming over WebSocket. Node types: triggers, logic, AI, HTTP, GitHub,
communication, variables.

**Not in v1:** microservices, Kubernetes, marketplace, billing, plugins,
mobile, 100+ integrations. Workspace teams, invitations, and four-role RBAC
are implemented in v1; custom roles, per-resource grants, nested groups, and
cross-workspace sharing remain out of scope. Future node types (Docker, SSH,
Kubernetes, Kafka, Terraform, …) must not influence MVP architecture.
