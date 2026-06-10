"""PulseOps ADK agent package.

Exposes `root_agent` at the package level so ADK's agent loader (used by
`adk run` / `adk web` / `adk deploy agent_engine`) discovers it directly.
"""

from .agent import root_agent  # noqa: F401  (ADK Agent Engine / CLI discovery)

__all__ = ["root_agent"]
