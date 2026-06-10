"""Live ADK + Gemini investigation runner (uses the Elastic MCP toolset).

Runs `root_agent` over the incident context, lets Gemini call the Elastic MCP
tools to gather evidence, then extracts and validates the InvestigationResult
JSON. Raises on any failure so the caller can fall back deterministically.
"""

from __future__ import annotations

import asyncio
import json
import logging

from ..prompts import build_investigation_prompt
from ..validators.schema_validator import normalize_and_validate_investigation_result

logger = logging.getLogger("pulseops.agent.adk_runner")

APP_NAME = "pulseops"
USER_ID = "pulseops-backend"


def run_agent_investigation(incident_context: dict) -> dict:
    """Synchronous entrypoint: run the agent and return a validated result."""
    return asyncio.run(_run_agent_investigation_async(incident_context))


async def _run_agent_investigation_async(incident_context: dict) -> dict:
    from google.adk.runners import InMemoryRunner
    from google.genai import types

    from ..agent import build_root_agent

    agent = build_root_agent()
    if agent is None:
        raise RuntimeError("ADK root agent unavailable")

    runner = InMemoryRunner(agent=agent, app_name=APP_NAME)
    session = await runner.session_service.create_session(app_name=APP_NAME, user_id=USER_ID)

    prompt = build_investigation_prompt(incident_context)
    message = types.Content(role="user", parts=[types.Part(text=prompt)])

    final_text = ""
    async for event in runner.run_async(
        user_id=USER_ID, session_id=session.id, new_message=message
    ):
        if event.is_final_response() and event.content and event.content.parts:
            final_text = "".join(part.text or "" for part in event.content.parts)

    if not final_text.strip():
        raise RuntimeError("empty model response")

    payload = _extract_json(final_text)
    return normalize_and_validate_investigation_result(payload)


def _extract_json(text: str) -> dict:
    """Extract the first JSON object from model text (tolerates markdown fences)."""
    start = text.find("{")
    end = text.rfind("}")
    if start == -1 or end == -1 or end <= start:
        raise ValueError("no JSON object in model response")
    return json.loads(text[start : end + 1])
