# Phase E3 — Security Validation & Rollback Drill

Operational security posture and rollback runbook for the
`pulseops-google-agent-adapter` Cloud Run service.

## Security checks

| Check | Control | Verified by |
|---|---|---|
| Inbound auth enforced | Bearer `INBOUND_AUTH_TOKEN` required on `/investigate`; missing token in `APP_ENV=production` is a fatal startup gate | `safety.CheckSecurityConfig`, adapter handler auth, `scripts/phase7-e3.ps1` (401/200 + prod-gate) |
| Token/secret redaction in logs | JSON logger redacts `INBOUND_AUTH_TOKEN`, `ELASTIC_MCP_TOKEN`, `AGENT_BUILDER_AUTH` values and sensitive keys (`authorization`, `token`, ...) | `obs` redaction unit tests + E3 log scan |
| Least-privilege IAM | Runtime SA holds only the four roles below | `scripts/phase7-e3.ps1 -GcpChecks` |
| No raw evidence dumps by default | Adapter logs `evidence_lines` count only — never `evidence_summary` or `prompt` content | `httpapi` no-dump test + E3 log scan |

### Expected runtime service-account roles (least privilege)

Runtime SA: `pulseops-agent-svc@<project>.iam.gserviceaccount.com`

- `roles/aiplatform.user`
- `roles/secretmanager.secretAccessor`
- `roles/logging.logWriter`
- `roles/monitoring.metricWriter`

`roles/run.invoker` is granted on the **service** (to the caller), not to the
runtime SA. Verify:

```bash
PROJECT=pulseops-agent
SA="pulseops-agent-svc@${PROJECT}.iam.gserviceaccount.com"
gcloud projects get-iam-policy "$PROJECT" \
  --flatten="bindings[].members" \
  --filter="bindings.members:serviceAccount:${SA}" \
  --format="value(bindings.role)"
```

Any role beyond the four listed is an over-grant and should be removed.

## Rollback drill (exit criteria: < 5 minutes, no contract break)

Cloud Run keeps every deployed revision, so rollback is a traffic shift — no
rebuild. Pin a known-good revision before promoting a candidate.

```bash
PROJECT=pulseops-agent
REGION=us-central1
SERVICE=pulseops-google-agent-adapter

# 1. Tag the currently-serving revision as the stable rollback target.
STABLE=$(gcloud run services describe "$SERVICE" --region "$REGION" --project "$PROJECT" \
  --format='value(status.traffic[0].revisionName)')
gcloud run services update-traffic "$SERVICE" --region "$REGION" --project "$PROJECT" \
  --set-tags "stable=$STABLE"

# 2. Shift 100% of traffic to the newest (candidate) revision.
gcloud run services update-traffic "$SERVICE" --region "$REGION" --project "$PROJECT" \
  --to-latest

# 3. Simulate a failure trigger / run the contract smoke against the live URL.
#    (scripts/phase7-e3.ps1 -RollbackDrill performs this automatically.)

# 4. Roll traffic back to the stable revision.
gcloud run services update-traffic "$SERVICE" --region "$REGION" --project "$PROJECT" \
  --to-revisions "$STABLE=100"
```

**Contract integrity:** before and after the rollback, POST the frozen request
fixture (`docs/contracts/adk_request_fixture.json`) to `<url>/investigate` and
confirm the response envelope still contains `request_id`, `trace_id`,
`status.transport`, and `status.workflow`. A passing smoke on both sides proves
the rollback caused no contract break.

Run the whole drill (timed) with:

```powershell
pwsh -NoProfile -File .\scripts\phase7-e3.ps1 -RollbackDrill `
  -Project pulseops-agent -Region us-central1 `
  -Service pulseops-google-agent-adapter -AuthToken <inbound-token>
```
