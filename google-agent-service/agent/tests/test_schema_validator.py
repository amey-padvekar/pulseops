from agent.agent import run_local_baseline
from agent.validators.schema_validator import validate_investigation_result


def test_validate_investigation_result_valid_payload():
    payload = {
        "probableCause": "service stopped",
        "confidence": 0.8,
        "recommendedActions": [{"actionId": "restart_service"}],
        "validationSteps": ["check service status"],
        "summary": "Service appears stopped.",
    }

    validate_investigation_result(payload)


def test_run_local_baseline_returns_investigation_result_shape():
    context = {
        "device_id": "dev-300",
        "service": "OpenVPNService",
        "evidence_summary": "heartbeat=true service_status=stopped",
    }

    result = run_local_baseline(context)

    # Validate required shape and constraints.
    validate_investigation_result(result)
    assert "probableCause" in result
    assert "confidence" in result
    assert "recommendedActions" in result
    assert "validationSteps" in result
    assert "summary" in result
