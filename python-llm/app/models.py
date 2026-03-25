"""Pydantic models for LLM output validation."""

from __future__ import annotations

import re
from typing import Any
from enum import Enum
from pydantic import BaseModel, field_validator, model_validator


class ArticleFormat(str, Enum):
    """Output format for the generated article."""

    THREAD = "thread"  # Multi-post technical thread
    NEWS = "news"  # Short, punchy light news post


class ArticleStatus(str, Enum):
    """Valid status values for articles."""

    PENDING = "pending"
    PROCESSING = "processing"
    TRIAGING = "triaging"
    GENERATING = "generating"
    POSTING = "posting"
    WAITING_APPROVAL = "waiting_approval"
    APPROVED = "approved"
    COMPLETED = "completed"
    POSTED = "posted"
    FAILED = "failed"
    PARTIAL_FAILED = "partial_failed"
    RETRY = "retry"
    IGNORED = "ignored"
    REJECTED_COMPLIANCE = "rejected_compliance"
    DRAFT = "draft"


class EvaluationMetric(BaseModel):
    metric_name: str
    score: float | str
    source_quote: str
    score_note: str | None = (
        ""  # Note for score transformations (e.g., "2x fewer -> 2x削減")
    )


class ExtractionResult(BaseModel):
    """Step 1: Extract objective facts."""

    arxiv_id: str | None = None
    model_size: str | None = ""  # e.g., "1.1B", "7B"
    target_hardware: str | None = ""
    primary_result: str | None = ""  # e.g., "1.5x-2.0x faster than FA2"
    problem_statement: str | None = ""
    proposed_architecture: str | None = ""
    technical_keyword: str | None = ""  # 1-2 keywords for core tech
    evaluation_metrics: list[EvaluationMetric] | None = []
    limitations: str | None = ""
    current_status: str | None = ""  # e.g., "Research Preview", "Release"
    extraction_confidence: float = 1.0  # Self-evaluation (0.0-1.0)
    selected_format: ArticleFormat = ArticleFormat.THREAD
    reason_for_insufficient: str | None = (
        ""  # Mandatory if is_information_sufficient is False
    )
    is_information_sufficient: bool = False


class ArticleResponse(BaseModel):
    """Step 2: Generate the final article."""

    content: str


class ArticleUpdate(BaseModel):
    """Model for updating article fields in the repository."""

    status: ArticleStatus | None = None
    hook_text: str | None = None
    article_body: str | None = None
    last_posted_index: int | None = None
    x_thread_ids: list[str] | None = None
    x_post_id: str | None = None


class TriggerResponse(BaseModel):
    """Response model for the /trigger endpoint."""

    status: str


# ---------------------------------------------------------------------------
# End of file
