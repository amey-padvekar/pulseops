"""Minimal schema validator for InvestigationResult scaffold."""

from __future__ import annotations

from copy import deepcopy
import re

from ..tools.action_tool import allowed_actions


class ValidationError(ValueError):
    """Raised when investigation result does not satisfy required shape."""


def validate_investigation_result(payload: dict, *, reject_unknown_action_ids: bool = True) -> None:
    required = [
        "probableCause",
        "confidence",
        "recommendedActions",
        "validationSteps",
        "summary",
    ]
    missing = [k for k in required if k not in payload]
    if missing:
        raise ValidationError(f"missing required fields: {', '.join(missing)}")

    confidence = payload.get("confidence")
    if not isinstance(confidence, (int, float)) or not 0.0 <= float(confidence) <= 1.0:
        raise ValidationError("confidence must be a number between 0.0 and 1.0")

    if not isinstance(payload.get("recommendedActions"), list):
        raise ValidationError("recommendedActions must be an array")

    approved_actions = set(allowed_actions())
    for item in payload["recommendedActions"]:
        if not isinstance(item, dict):
            raise ValidationError("recommendedActions entries must be objects")
        action_id = str(item.get("actionId") or "").strip()
        if not action_id:
            raise ValidationError("recommendedActions.actionId must be non-empty")
        if action_id not in approved_actions and reject_unknown_action_ids:
            raise ValidationError("recommendedActions contains unapproved actionId")
        if _contains_unsafe_text(item.get("target")) or _contains_unsafe_text(item.get("reason")):
            raise ValidationError("recommendedActions contains unsafe content")

    if not isinstance(payload.get("validationSteps"), list) or len(payload["validationSteps"]) == 0:
        raise ValidationError("validationSteps must be a non-empty array")
    for step in payload["validationSteps"]:
        if not isinstance(step, str) or not step.strip():
            raise ValidationError("validationSteps entries must be non-empty strings")
        if _contains_unsafe_text(step):
            raise ValidationError("validationSteps contains unsafe content")

    if not isinstance(payload.get("probableCause"), str) or not payload["probableCause"].strip():
        raise ValidationError("probableCause must be non-empty")
    if _contains_unsafe_text(payload["probableCause"]):
        raise ValidationError("probableCause contains unsafe content")

    if not isinstance(payload.get("summary"), str) or not payload["summary"].strip():
        raise ValidationError("summary must be non-empty")
    if _contains_unsafe_text(payload["summary"]):
        raise ValidationError("summary contains unsafe content")


def normalize_and_validate_investigation_result(payload: dict) -> dict:
    """Strip unknown action IDs, then validate strict schema and policy constraints."""
    normalized = deepcopy(payload)
    approved_actions = set(allowed_actions())

    actions = normalized.get("recommendedActions")
    if isinstance(actions, list):
        normalized["recommendedActions"] = [
            item
            for item in actions
            if isinstance(item, dict) and str(item.get("actionId") or "").strip() in approved_actions
        ]

    validate_investigation_result(normalized, reject_unknown_action_ids=True)
    return normalized


def _contains_unsafe_text(value: object) -> bool:
    if value is None:
        return False

    text = str(value)
    if not text.strip():
        return False

    low = text.lower()
    if any(ch in text for ch in ";|$`\\"):
        return True

    if "&&" in low:
        return True

    # Match command-like words with boundaries to avoid false positives like
    # "confirm" matching "rm ".
    command_patterns = [
        r"(^|\s)sudo(\s|$)",
        r"(^|\s)rm(\s|$)",
        r"(^|\s)curl(\s|$)",
        r"(^|\s)wget(\s|$)",
        r"(^|\s)bash(\s|$)",
        r"(^|\s)sh(\s|$)",
        r"(^|\s)powershell(\s|$)",
        r"(^|\s)cmd\.exe(\s|$)",
    ]

    return any(re.search(pattern, low) is not None for pattern in command_patterns)
