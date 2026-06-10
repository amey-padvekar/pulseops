"""Runtime configuration for the PulseOps ADK agent (Gemini + Elastic MCP).

Pure, dependency-free helpers so wiring can be unit-tested without ADK/mcp
installed. The chosen production path is Option A: the Elastic Serverless
*Agent Builder MCP endpoint* over streamable-HTTP with an ApiKey header.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Mapping, Optional

DEFAULT_MODEL = "gemini-2.5-pro"
DEFAULT_MCP_TIMEOUT_SECONDS = 60.0
DEFAULT_DOCKER_IMAGE = "docker.elastic.co/mcp/elasticsearch"
DEFAULT_AUTH_SCHEME = "ApiKey"
_TRUTHY = {"1", "true", "yes", "on"}

# Read/query-only allow-list for the Elastic Agent Builder MCP tool surface.
# Keeps Gemini focused and never exposes mutating tools (delete/update/create).
# The standalone-server names (search/esql/get_mappings/list_indices) also pass,
# so the same default works for the stdio transport. Override with
# ELASTIC_MCP_TOOL_FILTER ("all"/"*" = expose every tool).
DEFAULT_TOOL_FILTER = [
    "platform_core_search",
    "platform_core_execute_esql",
    "platform_core_generate_esql",
    "platform_core_get_index_mapping",
    "platform_core_list_indices",
    "platform_core_index_explorer",
    "platform_core_get_document_by_id",
    # standalone docker.elastic.co/mcp/elasticsearch names (stdio transport):
    "search",
    "esql",
    "get_mappings",
    "list_indices",
    "get_shards",
]


def _env(env: Optional[Mapping[str, str]]) -> Mapping[str, str]:
    return env if env is not None else os.environ


def load_dotenv_if_present(path: Optional[str] = None) -> None:
    """Best-effort load of a local `.env` for dev entrypoints (no-op in prod).

    Used by local_runner / mcp_smoke so secrets live in a gitignored
    google-agent-service/agent/.env. Agent Engine uses real env vars instead.
    """
    try:
        from dotenv import load_dotenv  # type: ignore
    except Exception:
        return
    if path is None:
        path = os.path.join(os.path.dirname(__file__), ".env")
    load_dotenv(path)


def model_name(env: Optional[Mapping[str, str]] = None) -> str:
    """Return the Gemini model id (env GEMINI_MODEL / AGENT_MODEL, else default)."""
    e = _env(env)
    return (e.get("GEMINI_MODEL") or e.get("AGENT_MODEL") or DEFAULT_MODEL).strip()


@dataclass(frozen=True)
class ElasticMcpConfig:
    """Resolved Elastic MCP connection settings."""

    transport: str  # "http" | "stdio" | "disabled"
    url: str
    api_key: str
    auth_scheme: str
    timeout_seconds: float
    docker_image: str
    es_url: str
    es_api_key: str
    ssl_skip_verify: bool
    tool_filter: list  # allow-list of MCP tool names; empty = no filter

    @classmethod
    def from_env(cls, env: Optional[Mapping[str, str]] = None) -> "ElasticMcpConfig":
        e = _env(env)
        transport = (e.get("ELASTIC_MCP_TRANSPORT") or "http").strip().lower()

        timeout_raw = (e.get("ELASTIC_MCP_TIMEOUT_SECONDS") or "").strip()
        try:
            timeout = float(timeout_raw) if timeout_raw else DEFAULT_MCP_TIMEOUT_SECONDS
        except ValueError:
            timeout = DEFAULT_MCP_TIMEOUT_SECONDS

        raw_filter = e.get("ELASTIC_MCP_TOOL_FILTER")
        if raw_filter is None:
            tool_filter = list(DEFAULT_TOOL_FILTER)
        elif raw_filter.strip().lower() in ("", "all", "*"):
            tool_filter = []  # expose every tool
        else:
            tool_filter = [t.strip() for t in raw_filter.split(",") if t.strip()]

        return cls(
            transport=transport,
            url=(e.get("ELASTIC_MCP_URL") or "").strip(),
            api_key=(e.get("ELASTIC_MCP_API_KEY") or "").strip(),
            auth_scheme=(e.get("ELASTIC_MCP_AUTH_SCHEME") or DEFAULT_AUTH_SCHEME).strip(),
            timeout_seconds=timeout,
            docker_image=(e.get("ELASTIC_MCP_DOCKER_IMAGE") or DEFAULT_DOCKER_IMAGE).strip(),
            es_url=(e.get("ES_URL") or "").strip(),
            es_api_key=(e.get("ES_API_KEY") or "").strip(),
            ssl_skip_verify=str(e.get("ES_SSL_SKIP_VERIFY") or "").strip().lower() in _TRUTHY,
            tool_filter=tool_filter,
        )

    def enabled(self) -> bool:
        """True when the configured transport has the credentials it needs."""
        if self.transport == "http":
            return bool(self.url and self.api_key)
        if self.transport == "stdio":
            return bool(self.es_url and self.es_api_key)
        return False

    def authorization_header(self) -> str:
        """Authorization header value, e.g. 'ApiKey <key>' (Elastic) or 'Bearer <key>'."""
        return f"{self.auth_scheme.strip()} {self.api_key}".strip()


def streamable_http_kwargs(cfg: ElasticMcpConfig) -> dict:
    """Build kwargs for ADK StreamableHTTPConnectionParams (ADK-free / testable)."""
    return {
        "url": cfg.url,
        "headers": {"Authorization": cfg.authorization_header()},
        "timeout": cfg.timeout_seconds,
    }


def stdio_docker_argv(cfg: ElasticMcpConfig) -> list[str]:
    """Build `docker` argv for the local stdio MCP server (ADK-free / testable)."""
    argv = ["run", "-i", "--rm", "-e", "ES_URL", "-e", "ES_API_KEY"]
    if cfg.ssl_skip_verify:
        argv += ["-e", "ES_SSL_SKIP_VERIFY"]
    argv += [cfg.docker_image, "stdio"]
    return argv
