"""Pydantic models for LLM output validation."""

import re
from pydantic import BaseModel, field_validator


class TriageResult(BaseModel):
    """Result of LLM triage — whether the content is valuable for SWE/AIE audience."""

    is_valuable: bool


class GeneratedArticle(BaseModel):
    """Generated article output from LLM, with strict validation rules."""

    hook_text: str
    article_body: str

    @field_validator("hook_text")
    @classmethod
    def hook_text_must_not_contain_url(cls, v: str) -> str:
        """URLs are forbidden to avoid X algorithm reach penalties."""
        if re.search(r"https?://", v):
            raise ValueError(
                "hook_text must not contain URLs (http:// or https://) "
                "to avoid X algorithm reach penalties"
            )
        return v

    @field_validator("article_body")
    @classmethod
    def article_body_must_not_contain_url(cls, v: str) -> str:
        """URLs are forbidden to avoid X algorithm reach penalties."""
        if re.search(r"https?://", v):
            raise ValueError(
                "article_body must not contain URLs (http:// or https://) "
                "to avoid X algorithm reach penalties"
            )
        return v

    @field_validator("article_body")
    @classmethod
    def article_body_min_length(cls, v: str) -> str:
        """Article body must meet minimum length requirement."""
        if len(v) < 1000:
            raise ValueError(
                f"article_body must be at least 1000 characters, got {len(v)}"
            )
        return v
