# PlayArena — Migration Validation Report

Covers migrations `000001` through `000014`.

---

## Migration Dependency Graph

Each migration is only safe to apply after all its listed dependencies have been applied.

```
000001_create_extensions_and_enums
  └── (root — no dependencies)

000002_create_organizations
  └── 000001  (org_status, org_type ENUMs)

000003_create_users
  └── 000001  (user_status, gender ENUMs)

000004_create_roles_and_permissions
  └── 000001  (role_scope ENUM)
  └── 000002  (organizations FK)
  └── 000003  (users FK × 2)

000005_create_players
  └── 000001  (player_status ENUM)
  └── 000002  (organizations FK)
  └── 000003  (users FK — nullable user_id)

000006_create_teams
  └── 000001  (team_status ENUM)
  └── 000002  (organizations FK)

000007_create_team_memberships
  └── 000001  (membership_role, membership_status ENUMs)
  └── 000005  (players FK)
  └── 000006  (teams FK)

000008_create_tournaments
  └── 000001  (tournament_status, tournament_format, participant_type ENUMs)
  └── 000002  (organizations FK)
  └── 000003  (users FK — nullable created_by)

000009_create_tournament_registrations
  └── 000001  (registration_status ENUM)
  └── 000002  (organizations FK)
  └── 000003  (users FK × 2 — nullable registered_by, approved_by)
  └── 000005  (players FK — nullable player_id)
  └── 000006  (teams FK — nullable team_id)
  └── 000008  (tournaments FK)

000010_create_matches
  └── 000001  (match_status ENUM)
  └── 000002  (organizations FK)
  └── 000005  (players FK × 4 — all nullable)
  └── 000006  (teams FK × 4 — all nullable)
  └── 000008  (tournaments FK)

000011_create_match_events
  └── 000001  (match_event_type ENUM)
  └── 000002  (organizations FK)
  └── 000003  (users FK — nullable recorded_by)
  └── 000005  (players FK — nullable player_id)
  └── 000006  (teams FK — nullable team_id)
  └── 000010  (matches FK)
  └── self    (match_events FK — cancels_event_id self-reference)

000012_create_media_attachments
  └── 000001  (media_entity_type, media_type ENUMs)
  └── 000002  (organizations FK)
  └── 000003  (users FK — nullable uploaded_by)

000013_create_audit_logs
  └── 000001  (audit_action ENUM)
  └── 000002  (organizations FK — nullable, SET NULL)
  └── 000003  (users FK — nullable, SET NULL)

000014_schema_hardening
  └── 000001–000013 (modifies constraints on tournament_registrations,
                     matches, match_events; adds column + triggers to
                     team_memberships; adds indexes across multiple tables)
```

---

## Execution Order

Apply migrations strictly in ascending numeric order. golang-migrate enforces
this automatically when you run `migrate up`.

```
000001 → 000002 → 000003 → 000004 → 000005 → 000006 → 000007
      → 000008 → 000009 → 000010 → 000011 → 000012 → 000013 → 000014
```

---

## Rollback Order

Roll back in exact reverse order. Never skip a step.

```
000014 → 000013 → 000012 → 000011 → 000010 → 000009 → 000008 → 000007
       → 000006 → 000005 → 000004 → 000003 → 000002 → 000001
```

---

## Potential Risks

### 000001 — Extensions and ENUMs
| Risk | Severity | Notes |
|------|----------|-------|
| `CREATE TYPE` has no `IF NOT EXISTS` in PostgreSQL | Low | golang-migrate tracks applied versions; re-running is prevented by the schema_migrations table |
| `DROP TYPE` in DOWN without `CASCADE` | Intentional | Will fail loudly if any table still references the type, forcing correct rollback order |

### 000011 — match_events (self-referential FK)
| Risk | Severity | Notes |
|------|----------|-------|
| `cancels_event_id` FK references the same table | Low | PostgreSQL handles self-referential FKs correctly; no special handling needed |

### 000014 — Schema Hardening ⚠️ Production-critical
| Risk | Severity | Mitigation |
|------|----------|------------|
| `CREATE INDEX` without `CONCURRENTLY` acquires `ShareLock` | **Medium** | Run during low-traffic window; or extract index statements and run manually with `CONCURRENTLY` outside a transaction |
| `ALTER TABLE … DROP CONSTRAINT / ADD CONSTRAINT` acquires `AccessExclusiveLock` | **Medium** | Each lock is held for milliseconds on an empty table; use `lock_timeout` on high-traffic systems: `SET lock_timeout = '5s'` before running |
| `UPDATE team_memberships SET organization_id = …` (backfill) | **Medium** | O(rows). Instantaneous on dev/empty tables. For production with >100k rows, run the UPDATE in batches before the migration |
| Trigger functions add a lookup per INSERT on `matches`, `match_events`, `tournament_registrations` | Low | Each trigger does a single indexed PK lookup; overhead is sub-millisecond with proper FK indexes in place |
| Rolling back 000014 drops `team_memberships.organization_id` | **High** | Data loss: the backfilled column is irrecoverable after rollback. Document and communicate before running DOWN in production |

---

## golang-migrate Commands

### Install

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### Environment variable (recommended)

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/playarena?sslmode=disable"
```

### Apply all pending migrations (up)

```bash
migrate -database "$DATABASE_URL" -path db/migrations up
```

Or via Makefile:

```bash
make migrate-up
```

### Roll back one migration (down 1)

```bash
migrate -database "$DATABASE_URL" -path db/migrations down 1
```

Or via Makefile:

```bash
make migrate-down
```

### Roll back all migrations

```bash
migrate -database "$DATABASE_URL" -path db/migrations down
```

```bash
make migrate-down-all
```

### Check current applied version

```bash
migrate -database "$DATABASE_URL" -path db/migrations version
```

```bash
make migrate-status
```

### Force a specific version

Use this **only** after manually fixing a dirty migration state.
A migration is marked dirty when it fails partway through.

```bash
# Syntax
migrate -database "$DATABASE_URL" -path db/migrations force <VERSION>

# Example: force to version 13 (removes the dirty flag at version 14)
migrate -database "$DATABASE_URL" -path db/migrations force 13
```

```bash
# Via Makefile
make migrate-force VERSION=13
```

> **Warning:** `force` does not run any SQL. It only updates the
> `schema_migrations` version record. Always inspect the database manually
> and repair partial state before forcing.

---

## Trigger Dependencies (000014)

The triggers added in 000014 have implicit runtime dependencies:

| Trigger | Table | Looks up | Required index |
|---------|-------|----------|----------------|
| `trg_matches_org_consistency` | `matches` | `tournaments(id)` | `pk_tournaments` (PK) |
| `trg_match_events_org_consistency` | `match_events` | `matches(id)` | `pk_matches` (PK) |
| `trg_treg_participant_org_consistency` | `tournament_registrations` | `teams(id, organization_id)` | `pk_teams` + `idx_teams_organization_id` |
| `trg_treg_participant_org_consistency` | `tournament_registrations` | `players(id, organization_id)` | `pk_players` + `idx_players_organization_id` |
| `trg_team_memberships_org_consistency` | `team_memberships` | `teams(id, organization_id)` | `pk_teams` + `idx_teams_organization_id` |
| `trg_team_memberships_org_consistency` | `team_memberships` | `players(id, organization_id)` | `pk_players` + `idx_players_organization_id` |

All required indexes are created in migrations 000005, 000006, and 000014.
The triggers are safe to apply in the order specified.
