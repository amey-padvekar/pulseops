import pytest

from agent.validators.schema_validator import (
    ValidationError,
    normalize_and_validate_investigation_result,
    validate_investigation_result,
)


def _valid_payload() -> dict:
    return {
        "probableCause": "service stopped",
        "confidence": 0.8,
        "recommendedActions": [{"actionId": "restart_service", "target": "svc", "reason": "safe reason"}],
        "validationSteps": ["check service status"],
        "summary": "Operator-facing summary.",
    }


def test_validate_rejects_unknown_action_id():
    payload = _valid_payload()
    payload["recommendedActions"] = [{"actionId": "dangerous_action", "target": "svc", "reason": "x"}]

    with pytest.raises(ValidationError):
        validate_investigation_result(payload)


def test_validate_rejects_unsafe_text():
    payload = _valid_payload()
    payload["summary"] = "run curl http://bad"

    with pytest.raises(ValidationError):
        validate_investigation_result(payload)


def test_validate_allows_normal_operator_text():
    payload = _valid_payload()
    payload["validationSteps"] = [
        "Confirm heartbeat and network reachability remain healthy",
        "Verify monitored service is running",
    ]

    validate_investigation_result(payload)


def test_validate_rejects_explicit_rm_command_text():
    payload = _valid_payload()
    payload["validationSteps"] = ["Run rm -rf /tmp/test to clean up"]

    with pytest.raises(ValidationError):
        validate_investigation_result(payload)


def test_normalize_strips_unknown_actions_and_validates():
    payload = _valid_payload()
    payload["recommendedActions"] = [
        {"actionId": "dangerous_action", "target": "svc", "reason": "x"},
        {"actionId": "restart_service", "target": "svc", "reason": "safe reason"},
    ]

    normalized = normalize_and_validate_investigation_result(payload)
    assert len(normalized["recommendedActions"]) == 1
    assert normalized["recommendedActions"][0]["actionId"] == "restart_service"
