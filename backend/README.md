# PlayArena Backend

This repository contains the Go backend scaffold for PlayArena.

## Structure

- `cmd/api` for the application entrypoint
- `internal/bootstrap` for composition and wiring
- `internal/platform` for shared infrastructure concerns
- `internal/*` for modular domain slices
- `db` for migrations, queries, and generated sqlc output

## Status

This is a minimal bootstrap only. Business logic, handlers, repositories, and module wiring are intentionally left as TODOs.
