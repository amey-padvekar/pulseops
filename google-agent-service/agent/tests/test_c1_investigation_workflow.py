from agent.workflows.investigate import run_investigation


def test_run_investigation_uses_assembled_evidence_for_confidence():
    payload = run_investigation(
        {
            "incident_id": "inc-9",
            "device_id": "dev-9",
            "service": "OpenVPNService",
            "evidence_summary": "heartbeat=true; serviceStatus=stopped",
            "elastic_mcp_enabled": False,
        }
    )

    assert payload["confidence"] >= 0.9
    assert len(payload["validationSteps"]) >= 3
