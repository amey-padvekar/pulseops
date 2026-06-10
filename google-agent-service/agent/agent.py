"""ADK root agent bootstrap for PulseOps investigation (Gemini + Elastic MCP).

Production path (Option A): the agent attaches the Elastic Serverless Agent
Builder MCP endpoint as an ADK `McpToolset` over streamable-HTTP, and Gemini
autonomously calls the Elastic tools to gather evidence before concluding.

The module is import-safe without ADK / the `mcp` package installed (so unit
tests run), and `root_agent` is defined synchronously at import time as Agent
Engine deployment requires.
"""

from __future__ import annotations

import logging

from .config import (
    ElasticMcpConfig,
    model_name,
    stdio_docker_argv,
    streamable_http_kwargs,
)
from .prompts import INVESTIGATION_SYSTEM_PROMPT, build_investigation_prompt
from .workflows.investigate import run_investigation

logger = logging.getLogger("pulseops.agent")

AGENT_NAME = "pulseops_investigator"

# ADK agents import is light; the MCP toolset import requires the `mcp` package.
try:
    from google.adk.agents import LlmAgent  # type: ignore

    _ADK_AGENTS = True
except Exception:  # pragma: no cover - import-safe without runtime deps
    LlmAgent = None  # type: ignore
    _ADK_AGENTS = False

try:
    from google.adk.tools.mcp_tool import (  # type: ignore
        McpToolset,
        StdioConnectionParams,
        StreamableHTTPConnectionParams,
    )

    _ADK_MCP = True
except Exception:  # pragma: no cover - `mcp` not installed
    McpToolset = None  # type: ignore
    StdioConnectionParams = None  # type: ignore
    StreamableHTTPConnectionParams = None  # type: ignore
    _ADK_MCP = False


def build_elastic_mcp_toolset(cfg: ElasticMcpConfig | None = None):
    """Return an ADK McpToolset for the Elastic MCP server, or None if disabled.

    http  -> Elastic Agent Builder MCP endpoint (Serverless), ApiKey header.
    stdio -> local `docker run ... docker.elastic.co/mcp/elasticsearch stdio`.
    """
    cfg = cfg or ElasticMcpConfig.from_env()
    if not cfg.enabled():
        logger.info("elastic mcp disabled (transport=%s)", cfg.transport)
        return None
    if not _ADK_MCP:
        raise RuntimeError(
            "Elastic MCP requested but the `mcp` package is not installed; "
            "install requirements.txt (adds `mcp`)."
        )

    tool_filter = cfg.tool_filter or None  # None = expose all tools

    if cfg.transport == "http":
        return McpToolset(
            connection_params=StreamableHTTPConnectionParams(**streamable_http_kwargs(cfg)),
            tool_filter=tool_filter,
        )

    if cfg.transport == "stdio":
        from mcp import StdioServerParameters  # type: ignore

        env = {"ES_URL": cfg.es_url, "ES_API_KEY": cfg.es_api_key}
        if cfg.ssl_skip_verify:
            env["ES_SSL_SKIP_VERIFY"] = "true"
        return McpToolset(
            connection_params=StdioConnectionParams(
                server_params=StdioServerParameters(
                    command="docker",
                    args=stdio_docker_argv(cfg),
                    env=env,
                ),
                timeout=cfg.timeout_seconds,
            ),
            tool_filter=tool_filter,
        )

    raise ValueError(f"unsupported ELASTIC_MCP_TRANSPORT: {cfg.transport}")


def build_root_agent(cfg: ElasticMcpConfig | None = None):
    """Return ADK root agent when ADK is available, else None for local scaffolding."""
    if not _ADK_AGENTS:
        return None
    cfg = cfg or ElasticMcpConfig.from_env()
    tools = []
    try:
        toolset = build_elastic_mcp_toolset(cfg)
        if toolset is not None:
            tools.append(toolset)
    except Exception as exc:  # pragma: no cover - depends on runtime env
        logger.warning("elastic mcp toolset unavailable: %s", exc)
    return LlmAgent(
        name=AGENT_NAME,
        model=model_name(),
        instruction=INVESTIGATION_SYSTEM_PROMPT,
        tools=tools,
    )


def run_local_baseline(incident_context: dict) -> dict:
    """Run the investigation workflow and return InvestigationResult-shaped JSON.

    Routes through run_investigation, which uses the live ADK+Gemini+MCP path when
    configured and falls back to a deterministic baseline otherwise.
    """
    _ = build_investigation_prompt(incident_context)
    return run_investigation(incident_context)


# Agent Engine requires root_agent to be defined synchronously at import time.
root_agent = build_root_agent()
