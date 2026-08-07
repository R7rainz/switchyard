# Switchyard

Switchyard is an AI-powered workflow automation platform for software teams.
Describe an engineering workflow in natural language, get an editable visual
graph, review it, and then execute it deterministically with visible logs.

AI drafts the route; the user still owns every node, connection, credential,
and execution.

<img width="2856" height="2328" alt="image" src="https://github.com/user-attachments/assets/7df74d4b-6a95-4a01-9f75-5dd396102e5a" />

## What Switchyard does

- Builds workflows visually with React Flow.
- Generates editable workflow graphs from natural-language prompts.
- Runs workflows asynchronously with status, timing, node outputs, errors,
  retries, and live WebSocket events.
- Keeps execution graph snapshots so a completed run always shows exactly what
  it executed.
- Gives teams workspace-scoped credentials, permissions, invitations, and
  encrypted secret storage.

Switchyard is intentionally focused on engineers, DevOps, platform teams, and
technical startups—not general-purpose marketing, HR, or sales automation.

## V1 delivered

### Workflow building and execution

- Workflow CRUD, graph validation, autosaving, drafts, duplication, version
  history, rollback, and reusable templates.
- Manual, scheduled, GitHub pull-request, and generic inbound-webhook triggers.
- Conditions, Switch, Delay, variables, HTTP requests, and branching.
- Chat, Summarize, Classification, Decision, and general AI nodes.
- Execution history, cancellation, explicit retry/recovery, startup reclaim of
  interrupted runs, idempotency keys, and live logs.

### AI and integrations

- OpenRouter is the default provider abstraction.
- Native OpenAI, Anthropic, and Google Gemini providers are supported through
  the same interface.
- GitHub pull-request reads, issue creation, comments, and merges.
- Slack messages, Discord webhook messages, and SMTP email messages.
- Signed GitHub webhooks and signed generic webhooks.

### Accounts, security, and storage

- Better Auth signup/login, JWT verification, sessions, and workspace RBAC.
- Viewer, Member, Admin, and Owner roles with membership and invitation rules.
- Encrypted credentials with key rotation; OAuth authorization-code flows store
  provider token documents through the credential service.
- Local artifact upload, download, and deletion APIs.
- SSRF protection, signed webhook validation, strict CORS, rate limiting, and
  migration verification.

### Frontend and API

- Dashboard, workflow builder, run viewer, settings, invitation flows, and
  landing page.
- Interactive workflow cards, duplication, version history/restore, template
  browsing, and save-as-template controls.
- Canonical REST API under `/api/v1`; `/api` remains a compatibility alias.
- REST and WebSocket communication between the Next.js frontend and Go API.

## Architecture

V1 is a modular monolith:

```text
Next.js + React Flow
        │ REST / WebSocket
        ▼
Go + Chi API
        │
        ├── Auth, workspace, credential, workflow, execution services
        ├── Workflow engine and integration runners
        └── PostgreSQL + local artifact storage
```

The backend is split into packages rather than deployable microservices. The
workflow engine does not know about HTTP, and integrations receive secrets only
through the credential service.

## V1 boundaries

- Storage is local filesystem in V1; an S3 backend is the next storage adapter.
- Billing and subscriptions are not implemented.
- Azure AI, browser automation, Docker, SSH, Kubernetes, databases, and other
  large integration families remain future work.
- Kubernetes and microservices are intentionally out of scope for V1.

## Local development

The frontend uses pnpm and the backend uses Go. Start PostgreSQL with the
project's Docker Compose setup, then run:

```bash
cd frontend
pnpm install
pnpm dev
```

In another terminal, configure `DATABASE_URL` and
`SWITCHYARD_CREDENTIAL_KEY`, then run:

```bash
cd backend
go run ./cmd/switchyard
```

Optional OAuth provider settings use `SWITCHYARD_OAUTH_*` variables, and local
artifact files default to `./data/artifacts` (override with
`SWITCHYARD_ARTIFACT_DIR`). See [`CLAUDE.md`](CLAUDE.md) for the complete
architecture and configuration decisions.

## Free deployment layout

For a test deployment, use Vercel for the Next.js app, Render for the Go API,
and Neon for PostgreSQL. Import `render.yaml` when creating the Render web
service; it sets the backend build command and `/healthz` health check.

Set these values in Vercel:

- `DATABASE_URL` — the Neon connection string
- `BETTER_AUTH_SECRET` — a new random secret
- `BETTER_AUTH_URL` — the deployed Vercel URL
- `NEXT_PUBLIC_API_URL` — the deployed Render API URL

Set these values in Render:

- `DATABASE_URL` — the same Neon connection string
- `SWITCHYARD_CREDENTIAL_KEY` — a versioned base64 AES-256 key
- `SWITCHYARD_AUTH_ISSUER` — the same deployed Vercel URL
- `SWITCHYARD_AUTH_AUDIENCE=switchyard-backend`
- `SWITCHYARD_ENV=production`

Render's free service sleeps when idle and has an ephemeral filesystem, so
local artifacts are suitable for testing only. Configure object storage before
depending on uploaded artifacts. Its free web services also cannot send SMTP
traffic, so the Email node needs a paid backend or an external mail relay that
supports an allowed transport.

## Verification

```bash
cd backend && go test ./... && go vet ./...
cd frontend && pnpm lint && pnpm build
```
