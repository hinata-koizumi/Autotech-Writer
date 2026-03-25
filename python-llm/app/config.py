"""Configuration for the Autotech Writer Python module."""

import os
from dataclasses import dataclass, field

from app.services.compliance import NG_KEYWORDS as _DEFAULT_NG_KEYWORDS


@dataclass
class DBConfig:
    url: str = field(
        default_factory=lambda: os.getenv(
            "DATABASE_URL", "postgresql://localhost:5432/autotech"
        )
    )
    pool_min_size: int = 1
    pool_max_size: int = 5


@dataclass
class LLMConfig:
    openai_api_key: str = field(default_factory=lambda: os.getenv("OPENAI_API_KEY", ""))
    anthropic_api_key: str = field(
        default_factory=lambda: os.getenv("ANTHROPIC_API_KEY", "")
    )

    # Model selections
    model_triage_openai: str = field(
        default_factory=lambda: os.getenv("MODEL_TRIAGE_OPENAI", "gpt-4o-mini")
    )
    model_gen_openai: str = field(
        default_factory=lambda: os.getenv("MODEL_GEN_OPENAI", "gpt-4o")
    )
    model_triage_anthropic: str = field(
        default_factory=lambda: os.getenv(
            "MODEL_TRIAGE_ANTHROPIC", "claude-3-haiku-20240307"
        )
    )
    model_gen_anthropic: str = field(
        default_factory=lambda: os.getenv(
            "MODEL_GEN_ANTHROPIC", "claude-3-5-sonnet-latest"
        )
    )

    # Full-text threshold (characters) to switch to structured full-text prompt
    fulltext_threshold: int = field(
        default_factory=lambda: int(os.getenv("FULLTEXT_THRESHOLD", "2000"))
    )

    # Defaults
    default_triage_provider: str = "openai"
    default_gen_provider: str = "openai"


@dataclass
class XConfig:
    api_key: str = field(default_factory=lambda: os.getenv("X_API_KEY", ""))
    api_secret: str = field(default_factory=lambda: os.getenv("X_API_SECRET", ""))
    access_token: str = field(default_factory=lambda: os.getenv("X_ACCESS_TOKEN", ""))
    access_secret: str = field(default_factory=lambda: os.getenv("X_ACCESS_SECRET", ""))

    thread_interval_seconds: float = field(
        default_factory=lambda: float(os.getenv("THREAD_INTERVAL_SECONDS", "5.0"))
    )


@dataclass
class LineConfig:
    channel_access_token: str = field(
        default_factory=lambda: os.getenv("LINE_CHANNEL_ACCESS_TOKEN", "")
    )
    channel_secret: str = field(
        default_factory=lambda: os.getenv("LINE_CHANNEL_SECRET", "")
    )
    user_id: str = field(default_factory=lambda: os.getenv("LINE_USER_ID", ""))


@dataclass
class Config:
    """Application configuration loaded from environment variables."""

    db: DBConfig = field(default_factory=DBConfig)
    llm: LLMConfig = field(default_factory=LLMConfig)
    x: XConfig = field(default_factory=XConfig)
    line: LineConfig = field(default_factory=LineConfig)

    # Human-in-the-loop approval
    approval_enabled: bool = field(
        default_factory=lambda: os.getenv("APPROVAL_ENABLED", "false").lower() == "true"
    )

    # Dry-run mode
    dry_run: bool = field(
        default_factory=lambda: os.getenv("DRY_RUN", "false").lower() == "true"
    )

    # Draft mode (Save to DB, don't post to X)
    stay_as_draft: bool = field(
        default_factory=lambda: os.getenv("STAY_AS_DRAFT", "true").lower() == "true"
    )

    # Article constraints
    min_article_length: int = field(
        default_factory=lambda: int(os.getenv("MIN_ARTICLE_LENGTH", "1000"))
    )

    # Compliance & Quality
    ng_keywords: list[str] = field(default_factory=lambda: list(_DEFAULT_NG_KEYWORDS))
    ai_ng_phrases: list[str] = field(
        default_factory=lambda: [
            "結論として",
            "まとめると",
            "さらに",
            "重要です",
            "この記事では",
            "つまり",
            "要するに",
            "つまりは",
        ]
    )

    # Retry config
    max_retries: int = 5

    # Inter-article processing delay (seconds) to avoid hammering LLM APIs
    article_interval_seconds: float = field(
        default_factory=lambda: float(os.getenv("ARTICLE_INTERVAL_SECONDS", "2.0"))
    )
