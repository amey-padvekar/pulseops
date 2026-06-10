"""Local runner for baseline / live verification without Agent Engine deployment.

By default this exercises the deterministic baseline. To drive the live
ADK + Gemini + Elastic MCP path locally, set `AGENT_INVESTIGATION_BACKEND=adk`
(or configure `ELASTIC_MCP_*`) and install requirements.txt — see README.md.
"""

from __future__ import annotations

import json

# Load a local .env (gitignored) BEFORE importing agent so root_agent picks up
# ELASTIC_MCP_* / GOOGLE_* for the live path.
from .config import load_dotenv_if_present

load_dotenv_if_present()

from .agent import run_local_baseline  # noqa: E402  (after dotenv load by design)


def sample_context() -> dict:
    return {
        "incident_id": "inc-200",
        "device_id": "dev-300",
        "service": "OpenVPNService",
        "evidence_summary": "heartbeat=true; serviceStatus=stopped; networkReachable=true",
        "elastic_context_hints": {
            "deviceId": "dev-300",
            "serviceName": "OpenVPNService",
            "incidentId": "inc-200",
            "indexPatterns": [
                "telemetry-events-*",
                "incident-events-*",
                "endpoint-logs-*",
            ],
        },
    }


def run_sample() -> dict:
    return run_local_baseline(sample_context())


if __name__ == "__main__":
    print(json.dumps(run_sample(), indent=2))
