"""Isolated Elastic MCP connectivity + tool-discovery smoke test (no Gemini).

Confirms ELASTIC_MCP_URL + ELASTIC_MCP_API_KEY are correct and the API key has
Agent Builder privileges, and prints the tool names Gemini will see. Run this
BEFORE the full agent run to isolate Elastic-side issues from model issues.

Usage:
    cd google-agent-service
    python -m agent.mcp_smoke
"""

from __future__ import annotations

import asyncio
import sys

from .config import ElasticMcpConfig, load_dotenv_if_present


async def _list_tools(cfg: ElasticMcpConfig) -> list[str]:
    from mcp import ClientSession
    from mcp.client.streamable_http import streamablehttp_client

    headers = {"Authorization": cfg.authorization_header()}
    async with streamablehttp_client(cfg.url, headers=headers) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            result = await session.list_tools()
            return [tool.name for tool in result.tools]


def main() -> int:
    load_dotenv_if_present()
    cfg = ElasticMcpConfig.from_env()

    if cfg.transport != "http":
        print(
            f"mcp_smoke targets the http transport; ELASTIC_MCP_TRANSPORT={cfg.transport}",
            file=sys.stderr,
        )
        return 2
    if not cfg.enabled():
        print(
            "ELASTIC_MCP_URL / ELASTIC_MCP_API_KEY not set (see agent/.env.example).",
            file=sys.stderr,
        )
        return 2

    print(f"Connecting to Elastic MCP: {cfg.url}")
    try:
        tools = asyncio.run(_list_tools(cfg))
    except Exception as exc:  # noqa: BLE001 - surface any connection/auth failure
        print(f"FAILED: {exc}", file=sys.stderr)
        print(
            "If 401/403: re-mint the API key with cluster monitor_inference + "
            "Kibana app privileges feature_agentBuilder.read + feature_actions.read.",
            file=sys.stderr,
        )
        return 1

    print(f"OK - {len(tools)} Elastic tools exposed via MCP:")
    for name in tools:
        print(f"  - {name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
