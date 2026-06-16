# Restore Drill — Results

**Mandatory PHI-1B evidence.** An untested restore does not count; this records a
real backup→destroy→restore round-trip executed by `deploy/scripts/restore-drill.sh`.

## How to reproduce
```sh
bash deploy/scripts/restore-drill.sh
```
Spins a throwaway Postgres 17, applies the real migrations (000001–000030), seeds
representative data, takes a `pg_dump -Fc` backup, **DROPs the database**, restores
from the dump, and verifies row counts + the standings and public-page queries.

## Result (latest run)

| Item | Value |
|---|---|
| Outcome | **PASS** |
| Representative data | 1 org · 2 teams · 1 tournament (ongoing, public) · 2 registrations · 1 completed match · 2 match events |
| Pre-backup counts | organizations=1, teams=2, tournaments=1, tournament_registrations=2, matches=1, match_events=2 |
| Post-restore counts | identical to pre-backup (all 6 tables) |
| **Actual data loss** | **none** (counts identical) |
| **Actual RTO** (destroy → restored + verified) | **~3 seconds** (disposable local PG; production RTO is dominated by provider restore time, target ≤ 60 min) |
| Standings feed query post-restore | 1/1 ✓ (completed match present) |
| Public-page gate query post-restore | 1/1 ✓ (org+tournament resolvable, visibility honored) |

## Interpretation
- The logical backup + `pg_restore` path round-trips **all** seeded data with zero
  loss and preserves the queries the app depends on (standings + public surface).
- The ~3 s RTO is the *mechanism* cost on a tiny dataset; for the pilot dataset on
  a managed instance, restore time is the provider's PITR/restore latency — well
  within the **≤ 60 min RTO** target. RPO is governed by Layer-1 PITR (≤ 5 min on
  tournament day).
- Re-run this drill **before each pilot** and whenever migrations change.
