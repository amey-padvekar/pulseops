from agent.agent import run_local_baseline


def test_run_local_baseline_returns_investigation_result_shape():
    payload = run_local_baseline(
        {
            "incident_id": "inc-1",
            "device_id": "dev-1",
            "service": "OpenVPNService",
            "evidence_summary": "heartbeat=true; serviceStatus=stopped",
        }
    )

    assert set(payload.keys()) == {
        "probableCause",
        "confidence",
        "recommendedActions",
        "validationSteps",
        "summary",
    }
    assert payload["probableCause"]
    assert 0.0 <= float(payload["confidence"]) <= 1.0
    assert isinstance(payload["recommendedActions"], list)
    assert isinstance(payload["validationSteps"], list)
    assert payload["summary"]
