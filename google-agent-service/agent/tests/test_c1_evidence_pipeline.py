from agent.workflows.evidence import (
    MAX_DOCS_PER_SOURCE,
    MAX_SNIPPET_CHARS,
    MAX_TOTAL_EVIDENCE_CHARS,
    assemble_evidence,
)


def test_assemble_evidence_enforces_bounds():
    long_doc = "x" * (MAX_SNIPPET_CHARS * 3)
    many_docs = [long_doc for _ in range(MAX_DOCS_PER_SOURCE + 30)]

    payload = {
        "incident_id": "inc-1",
        "device_id": "dev-1",
        "evidence_summary": "seed",
        "elastic_context_hints": {"incidentId": "inc-1"},
    }

    assembled = assemble_evidence(
        payload,
        elastic_enabled=True,
        elastic_fetch=lambda _: {"items": many_docs},
        incident_fetch=lambda _: {"state": "open", "severity": "high"},
        telemetry_fetch=lambda _: {"heartbeat": True, "serviceStatus": "stopped"},
    )

    assert assembled["source_counts"]["elastic"] <= MAX_DOCS_PER_SOURCE
    assert all(len(line) <= len("- [elastic] ") + MAX_SNIPPET_CHARS for line in assembled["evidence_lines"] if line.startswith("- [elastic]"))
    assert assembled["evidence_chars"] <= MAX_TOTAL_EVIDENCE_CHARS


def test_assemble_evidence_skips_elastic_when_disabled():
    payload = {
        "incident_id": "inc-2",
        "device_id": "dev-2",
        "evidence_summary": "heartbeat=true",
        "elastic_context_hints": {"incidentId": "inc-2"},
    }

    assembled = assemble_evidence(
        payload,
        elastic_enabled=False,
        elastic_fetch=lambda _: {"items": ["should-not-appear"]},
        incident_fetch=lambda _: {"state": "open"},
        telemetry_fetch=lambda _: {"heartbeat": True},
    )

    assert assembled["enrichment_used"] is False
    assert "elastic" not in assembled["source_counts"]
    assert "[elastic]" not in assembled["evidence_text"]


def test_assemble_evidence_fallback_when_enrichment_tools_fail():
    payload = {
        "incident_id": "inc-3",
        "device_id": "dev-3",
        "evidence_summary": "base evidence still available",
        "elastic_context_hints": {"incidentId": "inc-3"},
    }

    def boom(*_args, **_kwargs):
        raise RuntimeError("tool failure")

    assembled = assemble_evidence(
        payload,
        elastic_enabled=True,
        elastic_fetch=boom,
        incident_fetch=boom,
        telemetry_fetch=boom,
    )

    assert assembled["evidence_chars"] > 0
    assert "[provided]" in assembled["evidence_text"]
