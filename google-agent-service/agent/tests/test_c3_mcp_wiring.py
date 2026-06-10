"""W2 wiring tests: Elastic MCP config + connection params (no ADK/mcp needed)."""

from agent.agent import build_elastic_mcp_toolset
from agent.config import ElasticMcpConfig, stdio_docker_argv, streamable_http_kwargs


def test_http_config_builds_apikey_authorization_header():
    cfg = ElasticMcpConfig.from_env(
        {
            "ELASTIC_MCP_TRANSPORT": "http",
            "ELASTIC_MCP_URL": "https://kb.example.es.cloud/api/agent_builder/mcp",
            "ELASTIC_MCP_API_KEY": "SECRETKEY",
        }
    )
    assert cfg.enabled() is True
    kwargs = streamable_http_kwargs(cfg)
    assert kwargs["url"].endswith("/api/agent_builder/mcp")
    assert kwargs["headers"]["Authorization"] == "ApiKey SECRETKEY"
    assert kwargs["timeout"] > 0


def test_http_disabled_without_url_or_key():
    assert ElasticMcpConfig.from_env({"ELASTIC_MCP_TRANSPORT": "http"}).enabled() is False
    assert (
        ElasticMcpConfig.from_env(
            {"ELASTIC_MCP_TRANSPORT": "http", "ELASTIC_MCP_URL": "https://x/mcp"}
        ).enabled()
        is False
    )


def test_custom_auth_scheme_bearer():
    cfg = ElasticMcpConfig.from_env(
        {
            "ELASTIC_MCP_TRANSPORT": "http",
            "ELASTIC_MCP_URL": "https://x/api/agent_builder/mcp",
            "ELASTIC_MCP_API_KEY": "K",
            "ELASTIC_MCP_AUTH_SCHEME": "Bearer",
        }
    )
    assert streamable_http_kwargs(cfg)["headers"]["Authorization"] == "Bearer K"


def test_stdio_config_builds_docker_argv():
    cfg = ElasticMcpConfig.from_env(
        {
            "ELASTIC_MCP_TRANSPORT": "stdio",
            "ES_URL": "https://es.example.cloud:443",
            "ES_API_KEY": "ESKEY",
        }
    )
    assert cfg.enabled() is True
    argv = stdio_docker_argv(cfg)
    assert argv[0] == "run"
    assert "-i" in argv and "--rm" in argv
    assert "docker.elastic.co/mcp/elasticsearch" in argv
    assert argv[-1] == "stdio"


def test_toolset_is_none_when_disabled():
    # Degrades gracefully without ADK/mcp installed when no endpoint is configured.
    cfg = ElasticMcpConfig.from_env({"ELASTIC_MCP_TRANSPORT": "disabled"})
    assert build_elastic_mcp_toolset(cfg) is None


def test_default_transport_is_http():
    assert ElasticMcpConfig.from_env({}).transport == "http"


def test_tool_filter_defaults_to_core_read_tools():
    cfg = ElasticMcpConfig.from_env({})
    assert "platform_core_search" in cfg.tool_filter
    assert "platform_core_execute_esql" in cfg.tool_filter
    # mutating tools must never be in the default allow-list
    assert not any("delete" in t or "update" in t or "create" in t for t in cfg.tool_filter)


def test_tool_filter_all_disables_filter():
    assert ElasticMcpConfig.from_env({"ELASTIC_MCP_TOOL_FILTER": "all"}).tool_filter == []
    assert ElasticMcpConfig.from_env({"ELASTIC_MCP_TOOL_FILTER": "*"}).tool_filter == []


def test_tool_filter_custom_list():
    cfg = ElasticMcpConfig.from_env({"ELASTIC_MCP_TOOL_FILTER": "a, b ,c"})
    assert cfg.tool_filter == ["a", "b", "c"]
