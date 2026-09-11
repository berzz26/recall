# ReCall

ReCall is a system for natural language search over video.

Go handles the service and control plane. Python will handle the processing and compute plane.

This README covers Layer 1.1 and 1.2.

## What is implemented

Layer 1.1 provides the Go service bootstrap.

* Go with Fiber
* Health endpoint at GET /health
* Configuration from environment variables
* Structured logging with slog
* Graceful shutdown on SIGINT and SIGTERM
* Docker and Docker Compose support

Layer 1.2 adds PostgreSQL persistence.

* PostgreSQL with Docker Compose, health check and persistent volume
* pgx connection pool with verification and graceful close
* golang-migrate for migrations
* videos table with UUID primary key and status enum
* Module wise video package with model, dto, repository, service, handler and routes
* REST endpoints for videos metadata
* Health endpoint that checks the database

The repository uses direct pgx queries like foundry_api, not SQLC. Each domain owns its SQL.

## What is not implemented

These are left for later layers.

* Object storage
* Python workers
* FFmpeg, transcription, OCR, embeddings, vision logic
* Queues, Redis, Kafka, vector databases
* Real file upload handling
* Duration, FPS, codec or transcript fields

The structure is ready for those layers but they are not built yet.

## Project structure

```
recall/
├── pkg/
│   └── database/
│       └── postgres.go
├── services/
│   └── api/
│       ├── cmd/
│       │   └── server/
│       │       └── main.go
│       ├── internal/
│       │   ├── config/
│       │   │   └── config.go
│       │   ├── health/
│       │   │   └── handler.go
│       │   └── video/
│       │       ├── model.go
│       │       ├── dto.go
│       │       ├── repository.go
│       │       ├── service.go
│       │       ├── handler.go
│       │       └── routes.go
│       └── Dockerfile
├── workers/
│   └── processing/
├── migrations/
│   ├── 000001_create_videos.up.sql
│   └── 000001_create_videos.down.sql
├── docker-compose.yml
├── Makefile
├── go.mod
└── README.md
```

This follows the same module pattern as foundry_api where each domain has its own model, dto, repository, service, handler and routes, and the database package is shared in pkg/database.

## Prerequisites

* Go 1.25 or later
* Docker and Docker Compose
* make (optional but recommended)
* golang-migrate for development tasks

Install migrate if needed:

```bash
make migrate-install
```

## Configuration

Configuration is in services/api/internal/config and reads environment variables.

| Variable     | Default                                              | Description              |
|--------------|------------------------------------------------------|--------------------------|
| APP_ENV      | development                                          | Application environment  |
| APP_PORT     | 8080                                                 | HTTP port                |
| DATABASE_URL | postgres://recall:recall@localhost:5436/recall?sslmode=disable | Postgres connection |

You can set them in .env. Example .env:

```
APP_ENV=development
APP_PORT=8081
DATABASE_URL=postgres://recall:recall@localhost:5436/recall?sslmode=disable
```

Inside Docker Compose the API uses:

```
postgres://recall:recall@postgres:5432/recall?sslmode=disable
```

## Database schema

The initial migration creates:

* type video_status as enum with UPLOADING, UPLOADED, PROCESSING, READY, FAILED
* table videos with id uuid primary key, filename, content_hash, mime_type, size_bytes, status, created_at, updated_at

New videos start with status UPLOADING.

## How to run locally

Start PostgreSQL:

```bash
make db-up
```

Run migrations:

```bash
DATABASE_URL=postgres://recall:recall@localhost:5436/recall?sslmode=disable make migrate-up
```

Or directly:

```bash
migrate -path migrations -database "postgres://recall:recall@localhost:5436/recall?sslmode=disable" up
```

Start the API:

```bash
go run ./services/api/cmd/server
```

Or with make:

```bash
make run
```

The server starts on 8081 by default if you use the provided .env.

## How to run with Docker Compose

The compose stack has postgres and api. Postgres has a health check and a persistent volume pgdata. The API waits for postgres to be healthy.

```bash
docker compose up --build
```

Or with make:

```bash
make docker-up
```

The API will be at http://localhost:8081/health.

To run migrations inside the workflow, use the host DATABASE_URL with port 5436 after the postgres container is healthy.

To stop:

```bash
docker compose down
```

Or:

```bash
make docker-down
```

## API

Health:

```bash
curl http://localhost:8081/health
```

Response when database is up:

```json
{
  "status": "ok",
  "database": "ok"
}
```

If the database is down the endpoint returns 503:

```json
{
  "status": "error",
  "database": "unavailable"
}
```

Videos:

Create video metadata. This does not upload bytes yet. It only creates the database record.

```bash
curl -X POST http://localhost:8081/api/v1/videos \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "meeting.mp4",
    "content_hash": "abc123",
    "mime_type": "video/mp4",
    "size_bytes": 123456789
  }'
```

Response is 201 with the created record. Status will be UPLOADING.

List videos:

```bash
curl http://localhost:8081/api/v1/videos
```

Get one video:

```bash
curl http://localhost:8081/api/v1/videos/<id>
```

Delete video:

```bash
curl -X DELETE http://localhost:8081/api/v1/videos/<id>
```

Returns 204 on success, 404 if not found, 400 for invalid input.

## Makefile commands

```bash
make run            # run the server locally
make build          # build binary to bin/api
make tidy           # go mod tidy
make fmt            # go fmt
make test           # run all tests
make db-up          # start postgres container
make migrate-up     # apply migrations up
make migrate-down   # rollback one migration
make docker-build   # build docker image
make docker-up      # build and start compose stack
make docker-down    # stop compose stack
```

## Testing

Tests use the real PostgreSQL. Make sure postgres is running and migrated.

```bash
make db-up
make migrate-up
DATABASE_URL=postgres://recall:recall@localhost:5436/recall?sslmode=disable go test ./...
```

Or:

```bash
make test
```

Expected results:

* TestNew checks database connection and ping
* TestVideoCRUD creates, gets, lists, updates status, deletes
* TestServiceCreateAndGet checks service layer

If the database is not reachable the tests will be skipped with a clear message.

## Migrations

Migrations live in migrations and are managed by golang-migrate.

Create a new migration:

```bash
migrate create -ext sql -dir migrations -seq add_new_table
```

Apply:

```bash
make migrate-up
```

Rollback:

```bash
make migrate-down
```

Do not change the database outside migrations.

## Repository style

The video repository follows foundry_api style:

* Repository holds `*pgxpool.Pool`
* Each method builds SQL with `fmt.Sprintf` and `videoFields`
* Scanning is done with a shared `scanVideo` helper
* Service wraps repository and handler wraps service
* Each domain mounts its own Fiber router via `SetupRoutes`

This keeps SQL close to the domain and avoids generated code.

## Notes on architecture

Keep the boundary clean:

* Go is for API, orchestration, persistence and jobs
* Python is for media processing and ML and AI
* Object storage will hold the actual bytes

Do not add FFmpeg or ML code to the Go service. Keep handlers thin and keep SQL in repository. The flow is Handler to Service to Repository to PostgreSQL.
