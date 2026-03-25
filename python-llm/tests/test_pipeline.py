"""Tests for pipeline state transitions, retry logic, and triage flow."""

import asyncio
from unittest.mock import (
    AsyncMock,
    MagicMock,
    patch,
)

import pytest

from app.repository import ArticleRepository
from app.models import (
    ArticleStatus,
    ArticleUpdate,
)


# ============================================================
# 状態遷移テスト
# ============================================================


class TestStateTransitions:
    """Tests for article status state machine."""

    @pytest.mark.asyncio
    async def test_pending_to_processing(self):
        """[正常系] pending → processing へ正しくステータス更新できること"""
        mock_conn = AsyncMock()
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        await repo.update_status(
            1,
            ArticleUpdate(status=ArticleStatus.PROCESSING),
        )

        mock_conn.execute.assert_called_once()
        call_args = mock_conn.execute.call_args
        assert ArticleStatus.PROCESSING.value in call_args[0]

    @pytest.mark.asyncio
    async def test_processing_to_completed(self):
        """[正常系] processing → completed へ正しくステータス更新できること"""
        mock_conn = AsyncMock()
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        await repo.update_status(
            1,
            ArticleUpdate(
                status=ArticleStatus.COMPLETED,
                hook_text="テスト",
                article_body="本文" * 500,
                x_post_id="12345",
            ),
        )

        mock_conn.execute.assert_called_once()
        call_args = mock_conn.execute.call_args
        assert ArticleStatus.COMPLETED.value in call_args[0]

    @pytest.mark.asyncio
    async def test_processing_to_rejected_compliance(
        self,
    ):
        """[異常系] NGワード検出時に rejected_compliance へ更新できること"""
        mock_conn = AsyncMock()
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        await repo.update_status(
            1,
            ArticleUpdate(status=ArticleStatus.REJECTED_COMPLIANCE),
        )

        mock_conn.execute.assert_called_once()
        call_args = mock_conn.execute.call_args
        assert ArticleStatus.REJECTED_COMPLIANCE.value in call_args[0]


# ============================================================
# リトライカウント＋ステータス更新テスト
# ============================================================


class TestRetryCountManagement:
    """Tests for article retry count and failed status."""

    @pytest.mark.asyncio
    async def test_increment_retry_returns_retry_status(
        self,
    ):
        """[異常系] リトライ回数がmax未満の場合retryステータスが返ること"""
        mock_conn = AsyncMock()
        mock_conn.fetchval.return_value = ArticleStatus.RETRY.value
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        status = await repo.increment_retry(article_id=1, max_retries=5)
        assert status == ArticleStatus.RETRY

    @pytest.mark.asyncio
    async def test_increment_retry_to_failed(
        self,
    ):
        """[異常系] リトライ回数が上限到達でfailedステータスが返ること"""
        mock_conn = AsyncMock()
        mock_conn.fetchval.return_value = ArticleStatus.FAILED.value
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        status = await repo.increment_retry(article_id=1, max_retries=5)
        assert status == ArticleStatus.FAILED
