<img width="2856" height="2328" alt="image" src="https://github.com/user-attachments/assets/7df74d4b-6a95-4a01-9f75-5dd396102e5a" />

## V1 scope and status

Switchyard V1 follows the HLD's modular-monolith design: a Next.js frontend
communicates with a Go/Chi API over REST and WebSockets, with PostgreSQL as the
system of record and external integrations behind backend services.

### Planned V1 work delivered

- Better Auth signup/login, JWT verification, sessions, and protected API routes.
- Workspace-based collaboration with four-role RBAC, invitations, membership
  rules, and workspace-scoped encrypted credentials.
- Workflow CRUD, graph validation, React Flow builder, node configuration, and
  execution history.
- AI workflow generation through selectable OpenRouter, OpenAI, Anthropic, and
  Gemini providers, editable generated graphs, and user feedback capture.
- Deterministic execution with graph snapshots, status, timing, node outputs,
  errors, branching, variables, and live WebSocket events.
- Manual, scheduled, GitHub pull-request webhook, and workflow webhook trigger
  primitives.
- Working nodes for conditions, variables, HTTP requests, AI prompts, GitHub
  pull-request reads, Slack messages, Switch, Delay, Chat, Summarize,
  Classification, and Decision.
- Dashboard, workflow builder, run viewer, settings, invite flows, and landing
  page surfaces.

### Planned V1 work still pending

- A public generic inbound-webhook route; GitHub webhooks are implemented today.
- GitHub issue, comment, and merge nodes; Discord and Email integrations.
- Workflow duplication, version history/rollback, and a template library.
- OAuth credential flows and artifact/S3 storage.

Execution graph snapshots are an intentional replacement for a separate
`workflow_version` table: every run retains exactly what it executed.

## Remaining V1 TODO

- [ ] Add a public generic inbound-webhook route.
- [x] Add dedicated `Switch` and `Delay` nodes plus `Chat`, `Summarize`,
  `Classification`, and `Decision` AI nodes.
- [ ] Add GitHub issue, comment, and merge nodes, plus Discord and Email
  integrations.
- [ ] Add workflow duplication, version history/rollback, and a template
  library.
- [x] Add native OpenAI, Anthropic, and Gemini providers behind the shared AI
  interface.
- [ ] Add OAuth credential flows and artifact/S3 storage.

### Extra improvements added to V1

- Explicit retries for failed or cancelled runs, failed-run recovery, durable
  idempotency keys, and startup reclaim of abandoned executions.
- GitHub delivery deduplication and scheduler deduplication to prevent repeated
  side effects.
- SSRF protection for HTTP nodes, signed webhook validation, AI generation rate
  limiting, and strict CORS/auth boundaries.
- Versioned credential encryption and key rotation, migration verification, and
  additional execution/reliability tests.
- AI feedback persistence for improving generated workflows.
- Native AI provider adapters with per-node and per-generation provider
  selection; OpenRouter remains the default.
- Versioned backend API under `/api/v1`, with the existing `/api` path retained
  as a compatibility alias during migration.
- Versioned signed GitHub webhooks under `/api/v1/hooks/github/...`, with the
  original `/hooks/github/...` path retained for existing installations.
- Interactive pricing cards and WebGL landing-page effects. Billing and
  subscriptions are intentionally not implemented in V1.
