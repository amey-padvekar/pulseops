"""Action catalog tool scaffold."""


def allowed_actions() -> list[str]:
    """Return approved action IDs for phase baseline."""
    return ["restart_service", "flush_dns", "reconnect_vpn"]
