# TaskFlow

TaskFlow is a Go-based workflow platform designed to help teams coordinate work with clarity, visibility, and programmable automation. It combines a polished task dashboard with a secure, machine-readable API layer for external tools, automation workflows, and AI-assisted operations.

## Why this project matters

Modern teams increasingly rely on fragmented systems: dashboards, chat tools, automation agents, and external services that all need a common operational layer. TaskFlow addresses this gap by providing a structured workflow system that is both human-friendly and compatible with programmable integrations.

The project is built around a simple principle: operational work should be easy for people to manage and easy for software to access securely. This makes it a strong base for collaboration, automation, and future blockchain-aware payment workflows.

## Product overview

TaskFlow supports:

- task creation, updates, status transitions, and deletion
- project-based task organization for work streams and team delivery
- task detail views with a workflow-first interface
- file attachment support for operational context
- health and operational monitoring endpoints
- protected API access for machine consumers and external integrations
- a clean foundation for future identity, permission, and payment-enriched workflows

## Architecture

```text
Browser / UI
  ↓
Go HTTP server (main.go)
  ↓
Service layer (service/task.go)
  ↓
Storage abstraction (storage/task.go)
  ├── InMemoryTaskStore
  └── PostgreSQL-backed store (optional)

Protected API routes
  ↓
Middleware/x402 verification
```

## Key routes

- `/login` — sign in
- `/register` — create an account
- `/dashboard` — main task overview
- `/tasks` — task list and create flow
- `/tasks/{id}` — task details and updates
- `/upload` — file upload page
- `/profile` — user profile page
- `/docs` — project documentation and submission summary
- `/healthz` — health endpoint
- `/api/tasks` — x402-protected task creation endpoint
- `/api/projects/{id}/tasks` — x402-protected project task listing

## Quick start

```bash
go run main.go
```

Then open:

```text
http://localhost:8080/dashboard
```

For the docs page:

```text
http://localhost:8080/docs
```

## Environment configuration

TaskFlow will run with an in-memory store by default. To enable database persistence:

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/taskflow"
go run main.go
```

The project also supports operational tuning via:

```bash
export PORT=8080
export LOG_LEVEL=info
```

## API behavior

The JSON API is designed for external tools and agent systems. The current payment-gated endpoints are:

| Method | Route | Description |
|--------|-------|-------------|
| POST | `/api/tasks` | Create a new task |
| GET | `/api/projects/{id}/tasks` | List all tasks belonging to a project |
| PUT | `/api/tasks/{id}` | Update a task |
| DELETE | `/api/tasks/{id}` | Delete a task |

These endpoints enforce x402-style payment requirements and return `402 Payment Required` challenge responses when signatures or payment validation are missing or invalid.

## Security and trust model

The project is organized around a layered security model designed for both human users and automated clients:

- web routes are separate from API routes to keep user flows and machine workflows cleanly isolated
- service validation enforces core task rules before persistence
- middleware checks are isolated to request protection concerns
- API verification relies on cryptographic signature validation and x402 challenge logic
- responses remain structured and machine-readable for automation workflows and external tools

## Project structure

```text
Task-Flow/
├── main.go
├── dashboard.html
├── docs.html
├── login.html
├── register.html
├── profile.html
├── task-detail.html
├── upload.html
├── go.mod
├── README.md
├── middleware/
│   ├── verifier.go
│   ├── x402.go
│   └── x402_test.go
├── service/
│   ├── task.go
│   └── task_test.go
├── storage/
│   ├── task.go
│   ├── task_test.go
│   └── postgres.go
├── static/
│   ├── css/style.css
│   ├── images/
│   └── js/app.js
├── migrations/
│   └── 001_create_tasks.sql
├── scripts/
│   └── sign_payload.go
└── handlers_test.go
```

## Development notes

This codebase is intentionally organized to make progress straightforward:

- the router is centralized in `main.go`
- business rules live in `service/task.go`
- persistence is abstracted through `storage.TaskStore`
- signature and x402 logic live in `middleware/`
- tests cover core service and HTTP behavior

## Roadmap

Planned next steps include:

- real user authentication and secure password storage
- PostgreSQL-backed user and project models
- richer search, filtering, and reporting
- improved file handling and attachments storage
- expanded API client SDKs and integration examples
- stronger production observability and deployment automation

## Status

This project is a functional prototype and a strong foundation for a production-ready workflow tool. It is especially well-suited for demos, developer showcases, and grant or submission materials where a credible mix of product UX, backend architecture, and programmable API access is valued.

The codebase demonstrates a practical path from concept to deployable workflow platform, with room to evolve into a more complete collaboration layer as identity, permissions, and payment integrations mature.
