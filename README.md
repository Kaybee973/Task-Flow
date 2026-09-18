# TaskFlow

TaskFlow is a Go-based task management application built for clarity, operational visibility, and programmable automation. It combines a human-friendly dashboard with a payment-gated API layer for external integrations, AI tools, and agent-driven workflows.

## Why this project matters

Modern teams need a simple way to organize work, track status, and expose operational capabilities to tools without sacrificing trust or control. TaskFlow addresses this by combining:

- a clean browser-based task dashboard for team coordination
- structured API access for automation and external systems
- passwordless-like but secure session patterns for web access
- x402 payment gating for routes intended for external or paid access
- a service-oriented Go architecture that is easy to extend and deploy

## Product overview

TaskFlow supports:

- task creation, updates, status changes, and deletion
- project-based organization for work streams
- task detail pages and management UI
- file upload support for task-related artifacts
- health checks and operational monitoring
- protected API access for machine consumers via x402

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
- `/docs` — project documentation page
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

## Security approach

The project is organized around a layered security model:

- web routes are separate from API routes
- service validation enforces core task rules
- middleware checks are isolated to request protection concerns
- API verification uses cryptographic signature validation and x402 challenge logic
- responses remain structured and machine-readable for automation workflows

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

This project is a functional prototype and a solid foundation for a production-ready workflow tool. It is especially well-suited for demos, developer showcases, and submission materials where a credible mix of product UI and programmable API architecture is valued.
