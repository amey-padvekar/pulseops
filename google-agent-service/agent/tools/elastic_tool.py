"""Offline Elastic enrichment fallback.

NOTE: The real Elastic access now happens via the Elastic MCP server, which
Gemini calls directly through the ADK `McpToolset` (see agent.py / W2). This
function is only the offline/no-MCP fallback used by the deterministic baseline
in workflows/investigate.py; it intentionally returns no items.
"""


def fetch_elastic_context(hints: dict) -> dict:
    """Return empty evidence context for the offline (no-MCP) baseline path."""
    return {"source": "elastic", "hints": hints, "items": []}
