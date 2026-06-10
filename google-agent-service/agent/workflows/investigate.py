"""Investigation workflow entrypoint.

Primary path: live ADK + Gemini reasoning over Elastic MCP evidence.
Fallback: deterministic baseline so demo / offline / failure paths still produce
a valid InvestigationResult.
"""

from __future__ import annotations

import logging
import os

from ..config import ElasticMcpConfig
from ..tools.action_tool import allowed_actions
from ..validators.schema_validator import validate_investigation_result
from .evidence import assemble_evidence

logger = logging.getLogger("pulseops.agent.investigate")

_TRUTHY = {"1", "true", "yes", "on"}


def run_investigation(incident_context: dict) -> dict:
    """Create and validate an InvestigationResult from incident context."""
    if _live_mode_enabled():
        try:
            from .adk_runner import run_agent_investigation

            result = run_agent_investigation(incident_context)
            logger.info("investigation produced via ADK + Gemini + Elastic MCP")
            return result
        except Exception as exc:  # pragma: no cover - depends on runtime env
            logger.warning("ADK investigation failed; using deterministic fallback: %s", exc)

    return _deterministic_baseline(incident_context)


def _live_mode_enabled() -> bool:
    """Decide whether to run the live ADK path.

    Off for tests/offline by default: requires ADK + the `mcp` package installed
    and a configured Elastic MCP endpoint. `AGENT_INVESTIGATION_BACKEND` overrides
    (`adk` forces live, `offline` forces baseline).
    """
    if str(os.getenv("AGENT_FORCE_OFFLINE", "")).strip().lower() in _TRUTHY:
        return False

    backend = str(os.getenv("AGENT_INVESTIGATION_BACKEND", "auto")).strip().lower()
    if backend == "offline":
        return False

    try:
        # NOTE: `import google.adk.tools.mcp_tool` does NOT raise when `mcp` is
        # missing (ADK swallows it and leaves the module empty). Import the actual
        # symbol so a missing `mcp` package correctly disables live mode.
        from google.adk.agents import LlmAgent  # noqa: F401
        from google.adk.tools.mcp_tool import (  # noqa: F401
            McpToolset,
            StreamableHTTPConnectionParams,
        )
    except Exception:
        return False

    if backend == "adk":
        return True
    return ElasticMcpConfig.from_env().enabled()


def _deterministic_baseline(incident_context: dict) -> dict:
    """Deterministic InvestigationResult for offline / demo / fallback use."""
    elastic_enabled = str(
        incident_context.get("elastic_mcp_enabled", os.getenv("ELASTIC_MCP_ENABLED", "false"))
    ).lower() == "true"
    assembled = assemble_evidence(incident_context, elastic_enabled=elastic_enabled)

    evidence_summary = assembled["evidence_text"].lower()
    service = str(incident_context.get("service") or "service")
    target = str(incident_context.get("device_id") or service)
    action_id = "restart_service"

    probable_cause = "service stopped"
    confidence = 0.6
    if "heartbeat" in evidence_summary and "stopped" in evidence_summary:
        confidence = 0.9

    if action_id not in set(allowed_actions()):
        action_id = allowed_actions()[0]

    result = {
        "probableCause": probable_cause,
        "confidence": confidence,
        "recommendedActions": [
            {
                "actionId": action_id,
                "target": target,
                "reason": f"{service} appears stopped based on available evidence",
            }
        ],
        "validationSteps": [
            "Verify monitored service is running",
            "Confirm heartbeat and network reachability remain healthy",
            f"Review {len(assembled['evidence_lines'])} assembled evidence lines",
        ],
        "summary": "Evidence indicates service interruption and a restart is recommended.",
    }

    validate_investigation_result(result)
    return result
