# Runbook — Rollback

Revert a bad deploy quickly. **Code/image rollback is safe; database migrations
are NOT auto-rolled-back** (down-migrations can drop columns/data) — handle schema
separately and conservatively.

**Symptoms:** new errors / 5xx spike / broken behavior immediately after a deploy;
`APIUnavailable` or `HighErrorRate5xx` right after pushing a new image.

**Diagnosis**
```sh
docker compose -f deploy/docker-compose.prod.yml logs --tail=200 api web
# Confirm the regression correlates with the deploy timestamp.
```

**Immediate actions — image rollback**
- Pin to the previous known-good image tag and recreate:
  ```sh
  # tag images per release (e.g. playarena-api:<git-sha>). To roll back:
  docker compose -f deploy/docker-compose.prod.yml up -d \
    --no-deps api web    # after setting image: tags to the previous SHA
  ```
- If you only have `:latest`, redeploy from the previous git commit:
  `git checkout <prev-sha> && docker compose -f deploy/docker-compose.prod.yml up -d --build api web`.

**Database considerations**
- If the bad release added a migration:
  - **Additive migration (new column/table), app rolled back:** usually safe to
    leave the schema ahead of the code — do NOT down-migrate during an event.
  - **Destructive/incompatible migration:** prefer **PITR restore** (restore.md)
    to just before the deploy over running a down-migration, to avoid data loss.
- Never run `migrate down` on production during a live tournament without sign-off.

**Recovery actions**
- Confirm the previous version is healthy; open an incident note with the bad SHA
  so it isn't redeployed.

**Escalation**
- Rollback requires a destructive down-migration or PITR → platform owner.

**Verification**
- 5xx returns to baseline; the regressed flow works again; version/commit in logs
  matches the known-good build.
