"""Tests for full-text prompt switching logic."""

import pytest
from unittest.mock import AsyncMock, patch

from app.config import Config, LLMConfig
from app.services.llm.content import ContentService
from app.services.prompts import (
    EXTRACTION_SYSTEM_PROMPT,
    FULLTEXT_EXTRACTION_SYSTEM_PROMPT,
)


@pytest.fixture
def config():
    cfg = Config()
    cfg.llm = LLMConfig()
    cfg.llm.fulltext_threshold = 100  # Set low for testing
    cfg.dry_run = False
    return cfg


@pytest.mark.asyncio
async def test_extract_facts_uses_standard_prompt_for_short_content(config):
    """短文の場合は通常のプロンプトを使用することを確認"""
    service = ContentService(config)
    mock_provider = AsyncMock()
    mock_provider.call.return_value = '{"method": "test", "results": "test", "difference": "test", "categories": ["AI"]}'
    service.provider = mock_provider

    short_content = "This is a short summary."
    title = "Test Paper"

    await service.extract_facts(title=title, summary=short_content, source_type="arxiv")

    # Check that EXTRACTION_SYSTEM_PROMPT was used (formatted with raw_text)
    expected_prompt = EXTRACTION_SYSTEM_PROMPT.format(raw_text=short_content)
    mock_provider.call.assert_called_once()
    args, kwargs = mock_provider.call.call_args
    assert kwargs["system_prompt"] == expected_prompt
    assert kwargs["max_tokens"] == 4000


@pytest.mark.asyncio
async def test_extract_facts_uses_fulltext_prompt_for_long_content(config):
    """長文の場合はフルテキスト用のプロンプトを使用し、トークン上限が増えることを確認"""
    service = ContentService(config)
    mock_provider = AsyncMock()
    mock_provider.call.return_value = '{"method": "test", "results": "test", "difference": "test", "categories": ["AI"]}'
    service.provider = mock_provider

    # content length >= 100
    long_content = "This is a long content that exceeds the threshold. " * 5
    title = "Test Paper"

    await service.extract_facts(
        title=title, summary="summary", source_type="arxiv", full_content=long_content
    )

    # Check that FULLTEXT_EXTRACTION_SYSTEM_PROMPT was used (formatted with full_text)
    expected_prompt = FULLTEXT_EXTRACTION_SYSTEM_PROMPT.format(full_text=long_content)
    mock_provider.call.assert_called_once()
    args, kwargs = mock_provider.call.call_args
    assert kwargs["system_prompt"] == expected_prompt
    assert kwargs["max_tokens"] == 8000
