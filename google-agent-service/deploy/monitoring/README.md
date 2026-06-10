# Phase E2 — Observability & Alerting

This directory holds the deployable Cloud Monitoring artifacts for the
`pulseops-google-agent-adapter` Cloud Run service.

## Signal sources

The adapter emits two observability signals (no extra infra to scrape):

1. **Structured logs** — one `investigate_request` JSON line per request, with:
   `request_id, incident_id, device_id, trace_id, status_transport,
   status_workflow, latency_ms, confidence, action_ids, enrichment_used,
   evidence_lines, outcome`. Cloud Run ships these to Cloud Logging.
2. **Prometheus `/metrics` endpoint** — in-process counters and a latency
   histogram (with `investigate_latency_ms_p50` / `_p95` gauges) for direct
   scraping by Google Managed Prometheus or a local controlled test.

The Cloud Monitoring alerts and dashboard below are built on **log-based
metrics** derived from signal (1), so they work with the default Cloud Run
logging pipeline and require no scrape configuration.

## 1. Create the log-based metrics

```bash
PROJECT=pulseops-agent
FILTER_BASE='resource.type="cloud_run_revision" jsonPayload.message="investigate_request"'

# request counter
gcloud logging metrics create investigate_requests \
  --project="$PROJECT" \
  --description="PulseOps investigate requests" \
  --log-filter="$FILTER_BASE"

# error counter (any non-success terminal outcome)
gcloud logging metrics create investigate_errors \
  --project="$PROJECT" \
  --description="PulseOps investigate errors" \
  --log-filter="$FILTER_BASE jsonPayload.status_transport=\"error\""

# validation-failure counter
gcloud logging metrics create investigate_validation_fail \
  --project="$PROJECT" \
  --description="PulseOps investigate validation failures" \
  --log-filter="$FILTER_BASE jsonPayload.outcome=\"validation_fail\""

# timeout counter
gcloud logging metrics create investigate_timeout \
  --project="$PROJECT" \
  --description="PulseOps investigate timeouts" \
  --log-filter="$FILTER_BASE jsonPayload.outcome=\"timeout\""
```

The latency **distribution** metric extracts `latency_ms` and must be created
from a metric descriptor file (gcloud cannot set a value extractor inline):

```bash
gcloud logging metrics create investigate_latency_ms \
  --project="$PROJECT" \
  --config-from-file=latency-metric.yaml
```

`latency-metric.yaml`:

```yaml
name: investigate_latency_ms
description: PulseOps investigate latency (ms)
filter: resource.type="cloud_run_revision" jsonPayload.message="investigate_request"
metricDescriptor:
  metricKind: DELTA
  valueType: DISTRIBUTION
  unit: ms
valueExtractor: EXTRACT(jsonPayload.latency_ms)
bucketOptions:
  exponentialBuckets:
    numFiniteBuckets: 32
    growthFactor: 1.4
    scale: 10
```

## 2. Create alert policies and dashboard

```bash
gcloud monitoring channels list --project="$PROJECT"   # pick a notification channel id

gcloud alpha monitoring policies create --project="$PROJECT" \
  --policy-from-file=alert-error-rate.json \
  --notification-channels="$CHANNEL_ID"

gcloud alpha monitoring policies create --project="$PROJECT" \
  --policy-from-file=alert-p95-latency.json \
  --notification-channels="$CHANNEL_ID"

gcloud monitoring dashboards create --project="$PROJECT" \
  --config-from-file=dashboard.json
```

## 3. Controlled test (exit criteria)

Run `scripts/phase7-e2.ps1` to exercise the adapter locally and confirm the
metric set moves as expected (writes a summary artifact). To validate that the
**deployed** alerts fire:

- **Error-rate alert:** send a burst of malformed requests so the error ratio
  exceeds 5% for >10 minutes; confirm the policy opens an incident.
- **p95-latency alert:** point the adapter at a slow/stubbed ADK upstream (or
  set a deploy with induced delay) so p95 stays above 8000ms for >10 minutes;
  confirm the policy opens an incident.

Roll back induced load and confirm both incidents auto-close.
