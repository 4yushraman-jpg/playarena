# Runbook — Email Outage

Email = verification + password reset (onboarding front door). Delivery is async:
the API writes notification rows; the `EmailWorker` drains them via the provider.

**Symptoms:** `EmailWorkerStuck` (no ticks 10 m) or `EmailDeadLetterBurst` (>50
permanent failures); `OutboxBacklogHigh`; users report "no verification email".

**Diagnosis**
```sh
docker compose -f deploy/docker-compose.prod.yml logs --tail=200 api | grep -i email
# Provider dashboard (Resend/SES): are sends failing? domain still verified?
# Pilot Day dashboard: "Delivery backlog" panel (dead-letters / outbox pending).
```
- Worker not ticking → API/worker issue (see api-down). Dead-letter burst →
  provider rejecting (bad API key, unverified domain, rate cap, suppression).

**Immediate actions**
- Provider auth/domain: fix `EMAIL_SMTP_PASSWORD`/SES creds or re-verify
  SPF/DKIM/DMARC in Cloudflare DNS; recreate `api` to reload env.
- Hard outage with users waiting: **manually share links** — an organizer can be
  onboarded by sending them their verification/reset link out-of-band (support
  can read the pending link from the notification row if needed).

**Recovery actions**
- Once the provider is healthy the worker drains the backlog automatically; watch
  the outbox-pending panel return to ~0.

**Escalation**
- Provider-wide outage → switch `EMAIL_PROVIDER`/SMTP to the backup provider
  (SES↔Resend) in `.env.production`, recreate `api`.

**Verification**
- `EmailWorkerStuck` resolves; a test register → verification email lands in an
  inbox (not spam); dead-letter count stops growing.
