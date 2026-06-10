"""Evidence assembly pipeline for investigation reasoning."""

from __future__ import annotations

from time import monotonic
from typing import Any, Callable

from ..tools.elastic_tool import fetch_elastic_context
from ..tools.incident_tool import fetch_incident_context
from ..tools.telemetry_tool import fetch_telemetry_snapshot

MAX_DOCS_PER_SOURCE = 20
MAX_SNIPPET_CHARS = 500
MAX_TOTAL_EVIDENCE_CHARS = 8000


def assemble_evidence(
    incident_context: dict,
    *,
    elastic_enabled: bool,
    max_docs_per_source: int = MAX_DOCS_PER_SOURCE,
    max_snippet_chars: int = MAX_SNIPPET_CHARS,
    max_total_chars: int = MAX_TOTAL_EVIDENCE_CHARS,
    elastic_fetch: Callable[[dict], dict] | None = None,
    incident_fetch: Callable[[str], dict] | None = None,
    telemetry_fetch: Callable[[str], dict] | None = None,
) -> dict:
    """Assemble bounded evidence bullets for model reasoning."""
    elastic_fetch = elastic_fetch or fetch_elastic_context
    incident_fetch = incident_fetch or fetch_incident_context
    telemetry_fetch = telemetry_fetch or fetch_telemetry_snapshot

    lines: list[str] = []
    source_counts: dict[str, int] = {}
    total_chars = 0
    enrichment_used = False
    start_ts = monotonic()

    def enrichment_budget_exhausted() -> bool:
        return (monotonic() - start_ts) >= 3.0

    def add_docs(source: str, docs: list[Any]) -> None:
        nonlocal total_chars
        if not docs:
            return

        source_counts[source] = 0
        for raw in docs[:max_docs_per_source]:
            snippet = _coerce_snippet(raw)
            if not snippet:
                continue
            snippet = snippet[:max_snippet_chars]
            line = f"- [{source}] {snippet}"

            if total_chars + len(line) + 1 > max_total_chars:
                remaining = max_total_chars - total_chars
                if remaining <= 4:
                    return
                line = line[: remaining - 3] + "..."
                lines.append(line)
                total_chars += len(line) + 1
                source_counts[source] += 1
                return

            lines.append(line)
            total_chars += len(line) + 1
            source_counts[source] += 1

    evidence_summary = str(incident_context.get("evidence_summary") or "").strip()
    if evidence_summary:
        add_docs("provided", [evidence_summary])

    incident_id = str(incident_context.get("incident_id") or "").strip()
    device_id = str(incident_context.get("device_id") or "").strip()

    if incident_id and not enrichment_budget_exhausted():
        try:
            incident_ctx = incident_fetch(incident_id)
            add_docs("incident", [incident_ctx])
        except Exception:
            # Fallback policy: continue with base evidence when enrichment fails.
            pass

    if device_id and not enrichment_budget_exhausted():
        try:
            telemetry_ctx = telemetry_fetch(device_id)
            add_docs("telemetry", [telemetry_ctx])
        except Exception:
            pass

    hints = incident_context.get("elastic_context_hints") or {}
    if elastic_enabled and isinstance(hints, dict) and hints and not enrichment_budget_exhausted():
        try:
            elastic_ctx = elastic_fetch(hints)
            items = elastic_ctx.get("items") if isinstance(elastic_ctx, dict) else None
            if isinstance(items, list):
                add_docs("elastic", items)
                enrichment_used = len(items) > 0
        except Exception:
            pass

    evidence_text = "\n".join(lines)
    return {
        "evidence_lines": lines,
        "evidence_text": evidence_text,
        "evidence_chars": len(evidence_text),
        "enrichment_used": enrichment_used,
        "source_counts": source_counts,
    }


def _coerce_snippet(raw: Any) -> str:
    if raw is None:
        return ""
    if isinstance(raw, str):
        return " ".join(raw.split())
    if isinstance(raw, dict):
        parts = []
        for k in sorted(raw.keys()):
            val = raw[k]
            if val is None:
                continue
            parts.append(f"{k}={val}")
        return " ".join(parts)
    return " ".join(str(raw).split())
