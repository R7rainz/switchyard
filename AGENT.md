# AGENT.md

# Project: Switchyard

> An AI-powered workflow automation platform built for software teams.
>
> _AI drafts the route. You throw the switches._

---

# Vision

Switchyard is a workflow automation platform that combines the speed of AI with the reliability of visual workflows.

Instead of forcing users to manually create automation pipelines or blindly trusting AI-generated actions, Switchyard lets users describe a workflow in natural language, generates an editable workflow graph, and gives them complete control before execution.

The platform is built specifically for developers, engineering teams, DevOps engineers, and technical startups—not as a general-purpose automation platform like Zapier.

The goal is to make software engineering workflows programmable, observable, and easy to automate.

---

# Problem

Current workflow automation tools have several shortcomings.

### Traditional workflow builders

Examples:

- n8n
- Zapier
- Make

Pros

- Reliable
- Deterministic
- Easy to debug

Cons

- Slow to build
- Large learning curve
- Repetitive
- Difficult for new users

---

### AI-first workflow builders

Pros

- Fast
- Natural language interface
- Beginner friendly

Cons

- Black box execution
- Difficult to debug
- AI makes mistakes
- Users don't understand what was generated

---

Switchyard combines both approaches.

The workflow always becomes a visual graph that the user can inspect, modify, and execute.

AI accelerates workflow creation instead of replacing developer control.

---

# Target Audience

Primary users

- Software engineers
- Backend developers
- DevOps engineers
- Platform engineering teams
- Startup founders
- Technical teams

Not intended for

- Marketing automation
- HR workflows
- Sales pipelines
- No-code business automation

The platform is intentionally opinionated toward engineering workflows.

---

# Core Principles

## AI assists.

AI never owns the workflow.

The user owns the workflow.

Every generated action should be editable.

---

## Explainability

Every execution should be transparent.

Users should know

- what happened
- why it happened
- what data was used
- which AI response was generated
- which node failed

No hidden reasoning.

---

## Deterministic Execution

Once a workflow is saved, execution should always be deterministic unless the user intentionally changes it.

---

## Developer First

The platform should feel like software developers built it.

Developers should never feel constrained by the UI.

---

## Simplicity

Avoid unnecessary complexity.

Only introduce infrastructure when there is a real need.

---

# High Level Architecture

                    Next.js Frontend
                            │
                    REST + WebSocket
                            │
                        Go Backend
                            │
            ┌───────────────┴───────────────┐
            │                               │
      Workflow Engine                 AI Service
            │                               │
            └───────────────┬───────────────┘
                        PostgreSQL

Version 1 intentionally uses a modular monolith.

Microservices are not a goal.

---

# Technology Stack

## Frontend

- Next.js
- TypeScript
- Tailwind CSS
- shadcn/ui
- React Flow
- Zustand
- TanStack Query

---

## Backend

- Go
- Chi Router
- PostgreSQL
- WebSockets

---

## AI

Provider abstraction.

Possible providers

- OpenAI
- Anthropic
- Google Gemini

The application should never depend directly on one AI provider.

---

## Deployment

Docker Compose

No Kubernetes initially.

---

# MVP Features

## Authentication

- Sign Up
- Login
- Sessions

---

## Dashboard

Users can

- Create workflows
- Edit workflows
- Delete workflows
- Duplicate workflows
- Execute workflows
- View execution history

---

## Workflow Builder

Visual drag-and-drop editor.

Users can

- Create nodes
- Connect nodes
- Remove nodes
- Save graph
- Execute graph

---

## AI Workflow Generator

Example prompt

"When someone opens a GitHub pull request,
run tests,
summarize the changes,
and notify Slack."

↓

AI generates a workflow graph.

↓

User edits it.

↓

Save.

---

## Workflow Execution

Every workflow execution becomes an Execution.

Execution has

- Status
- Logs
- Start time
- Finish time
- Duration

---

## Live Logs

Users should see workflow execution live.

Example

09:30 Trigger received

09:30 Reading GitHub PR

09:31 Asking AI

09:31 AI responded

09:31 Posting Slack message

09:31 Workflow completed

---

# Initial Node Types

## Trigger

- Manual Trigger
- GitHub Webhook
- Schedule (Cron)

---

## Logic

- If
- Switch
- Delay

---

## AI

- Chat
- Summarize
- Classification
- Decision

---

## HTTP

- GET
- POST
- PUT
- DELETE

---

## GitHub

- Read Pull Request
- Create Issue
- Comment
- Merge

---

## Communication

- Slack
- Discord
- Email

---

## Variables

- Workflow Variables
- Environment Variables
- AI Outputs

---

# Future Node Types

- Playwright Browser
- Docker
- SSH
- Kubernetes
- Database
- Redis
- Kafka
- Git
- Terraform

These are future additions and should not influence the MVP architecture.

---

# Execution Engine

The execution engine is the heart of the platform.

Responsibilities

- Execute nodes
- Track state
- Handle failures
- Record logs
- Store outputs
- Pass variables
- Handle retries
- Support branching

The engine should be designed independently of the UI.

The frontend visualizes workflows.

The backend executes workflows.

---

# Folder Philosophy

Separate business logic from transport layers.

Avoid placing logic inside HTTP handlers.

Business logic belongs inside internal packages.

## Repository Layout

Two top-level directories, one per toolchain. No monorepo tooling — there is
exactly one frontend app, so a workspace would be ceremony without payoff.

    switchyard/
      AGENT.md
      backend/            Go module: github.com/R7rainz/switchyard/backend
      frontend/           Next.js app

## Backend

    backend/
      go.mod
      cmd/
        switchyard/       main package; wiring and startup only
      internal/
        config/           environment loading; the only reader of env vars
        api/              HTTP transport: routing, decode, encode, middleware
        auth/             signup, login, sessions, password hashing
        workflow/         graph model, validation, persistence
        execution/        the engine: runs nodes, tracks state, records logs
        ai/               provider abstraction; generation and AI nodes
        github/           pull requests, issues, comments, webhooks
        slack/            outbound messages
        websocket/        live execution log streaming
        database/         PostgreSQL connection and queries
      migrations/         SQL schema migrations
      pkg/                reusable code with no Switchyard-specific meaning

Every package carries a `doc.go` stating its responsibility. If a package's
purpose cannot be written in two sentences, it is doing too much.

`pkg/` is currently empty and stays that way until something genuinely
general-purpose earns a place in it. An empty `pkg/` is not a goal.

The transport package is named `api`, not `http`, so it never shadows the
standard library's `net/http` at a call site.

## Frontend

    frontend/
      src/
        app/              Next.js App Router: routes, layouts
        components/       React components
        lib/              client helpers, API bindings
      public/             static assets

## Dependency Direction

    api  ->  auth, workflow, execution
    execution  ->  ai, github, slack, database, websocket
    everything  ->  database, config

Domain packages never import `api`. The engine never imports HTTP. Arrows
point one way only; a cycle means a boundary was drawn wrong.

---

# Design Philosophy

The project should prioritize

- Readability
- Maintainability
- Explicit code
- Small interfaces
- Composition over inheritance
- Clear package boundaries

Avoid clever code.

Prefer boring code that is easy to understand.

---

# Non Goals

Version 1 should NOT include

- Microservices
- Kubernetes
- Marketplace
- Billing
- Teams
- RBAC
- Plugin marketplace
- Mobile application
- 100+ integrations

Build only what is necessary.

---

# Long Term Vision

Switchyard should become an automation platform where developers can automate engineering workflows using AI without sacrificing transparency or control.

The platform should eventually support

- AI-generated workflows
- Visual editing
- Live execution
- Workflow versioning
- Scheduling
- Team collaboration
- Browser automation
- Deployment automation
- Infrastructure automation

Every feature should reinforce one principle:

**AI should accelerate developers, not replace their understanding.**
