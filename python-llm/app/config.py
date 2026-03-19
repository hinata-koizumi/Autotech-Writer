"""Configuration for the Autotech Writer Python module."""

import os
from dataclasses import dataclass, field


@dataclass
class Config:
    """Application configuration loaded from environment variables."""

    # Database
    database_url: str = field(
        default_factory=lambda: os.getenv("DATABASE_URL", "postgresql://localhost:5432/autotech")
    )

    # LLM APIs
    openai_api_key: str = field(default_factory=lambda: os.getenv("OPENAI_API_KEY", ""))
    anthropic_api_key: str = field(default_factory=lambda: os.getenv("ANTHROPIC_API_KEY", ""))

    # X API
    x_api_key: str = field(default_factory=lambda: os.getenv("X_API_KEY", ""))
    x_api_secret: str = field(default_factory=lambda: os.getenv("X_API_SECRET", ""))
    x_access_token: str = field(default_factory=lambda: os.getenv("X_ACCESS_TOKEN", ""))
    x_access_secret: str = field(default_factory=lambda: os.getenv("X_ACCESS_SECRET", ""))

    # Dry-run mode
    dry_run: bool = field(
        default_factory=lambda: os.getenv("DRY_RUN", "false").lower() == "true"
    )
    dry_run_webhook_url: str = field(
        default_factory=lambda: os.getenv("DRY_RUN_WEBHOOK_URL", "")
    )

    # Article constraints
    min_article_length: int = 1000
    max_article_length: int = 3000

    # Retry config
    max_retries: int = 5
    base_delay: float = 1.0
