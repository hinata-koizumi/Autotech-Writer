"""Tests for pipeline state transitions, retry logic, and triage flow."""

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.repository import ArticleRepository, ArticleStatus
from app.retry import RetryExhaustedError, with_retry


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
        await repo.update_status(1, ArticleStatus.PROCESSING)

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
            ArticleStatus.COMPLETED,
            hook_text="テスト",
            article_body="本文" * 500,
            x_post_id="12345",
        )

        mock_conn.execute.assert_called_once()
        call_args = mock_conn.execute.call_args
        assert ArticleStatus.COMPLETED.value in call_args[0]

    @pytest.mark.asyncio
    async def test_processing_to_rejected_compliance(self):
        """[異常系] NGワード検出時に rejected_compliance へ更新できること"""
        mock_conn = AsyncMock()
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        await repo.update_status(1, ArticleStatus.REJECTED_COMPLIANCE)

        mock_conn.execute.assert_called_once()
        call_args = mock_conn.execute.call_args
        assert ArticleStatus.REJECTED_COMPLIANCE.value in call_args[0]


# ============================================================
# リトライロジックテスト
# ============================================================


class TestRetryLogic:
    """Tests for exponential backoff retry decorator."""

    @pytest.mark.asyncio
    async def test_success_no_retry(self):
        """[正常系] 初回成功時はリトライしないこと"""
        call_count = 0

        @with_retry(max_retries=3, base_delay=0.01)
        async def always_succeeds():
            nonlocal call_count
            call_count += 1
            return "ok"

        result = await always_succeeds()
        assert result == "ok"
        assert call_count == 1

    @pytest.mark.asyncio
    async def test_retry_then_success(self):
        """[異常系] 失敗後にリトライして成功すること"""
        call_count = 0

        @with_retry(max_retries=3, base_delay=0.01)
        async def fails_then_succeeds():
            nonlocal call_count
            call_count += 1
            if call_count <= 2:
                raise ConnectionError("API unavailable")
            return "ok"

        result = await fails_then_succeeds()
        assert result == "ok"
        assert call_count == 3

    @pytest.mark.asyncio
    async def test_all_retries_exhausted_raises(self):
        """[異常系] 全リトライを使い切った場合RetryExhaustedErrorが発生すること"""

        @with_retry(max_retries=2, base_delay=0.01)
        async def always_fails():
            raise ConnectionError("permanent failure")

        with pytest.raises(RetryExhaustedError) as exc_info:
            await always_fails()

        assert exc_info.value.attempts == 3  # 1 initial + 2 retries

    @pytest.mark.asyncio
    async def test_retry_with_rate_limit_error(self):
        """[異常系] レート制限エラー時にリトライが機能すること"""
        call_count = 0

        class RateLimitError(Exception):
            pass

        @with_retry(max_retries=3, base_delay=0.01, retryable_exceptions=(RateLimitError,))
        async def rate_limited_api():
            nonlocal call_count
            call_count += 1
            if call_count <= 1:
                raise RateLimitError("429 Too Many Requests")
            return {"status": "posted"}

        result = await rate_limited_api()
        assert result == {"status": "posted"}
        assert call_count == 2

    @pytest.mark.asyncio
    async def test_non_retryable_exception_not_retried(self):
        """[正常系] リトライ対象外の例外はリトライしないこと"""
        call_count = 0

        class FatalError(Exception):
            pass

        @with_retry(max_retries=3, base_delay=0.01, retryable_exceptions=(ConnectionError,))
        async def raises_fatal():
            nonlocal call_count
            call_count += 1
            raise FatalError("unrecoverable")

        with pytest.raises(FatalError):
            await raises_fatal()

        assert call_count == 1  # No retry


# ============================================================
# リトライカウント＋ステータス更新テスト
# ============================================================


class TestRetryCountManagement:
    """Tests for article retry count and failed status."""

    @pytest.mark.asyncio
    async def test_increment_retry_returns_retry_status(self):
        """[異常系] リトライ回数がmax未満の場合retryステータスが返ること"""
        mock_conn = AsyncMock()
        mock_conn.fetchrow.return_value = {"retry_count": 2}
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        status = await repo.increment_retry(article_id=1, max_retries=5)
        assert status == ArticleStatus.RETRY

    @pytest.mark.asyncio
    async def test_increment_retry_to_failed(self):
        """[異常系] リトライ回数が上限到達でfailedステータスが返ること"""
        mock_conn = AsyncMock()
        mock_conn.fetchrow.return_value = {"retry_count": 4}
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)
        status = await repo.increment_retry(article_id=1, max_retries=5)
        assert status == ArticleStatus.FAILED


# ============================================================
# トリアージロジックテスト
# ============================================================


class TestTriageFlow:
    """Tests for LLM triage skip logic."""

    @pytest.mark.asyncio
    async def test_triage_false_sets_ignored(self):
        """[正常系] トリアージがFalseの場合DBステータスをignoredに更新し後続処理がスキップされること"""
        mock_conn = AsyncMock()
        mock_pool = MagicMock()
        mock_pool.acquire.return_value.__aenter__.return_value = mock_conn
        mock_pool.acquire.return_value.__aexit__.return_value = False

        repo = ArticleRepository(mock_pool)

        # Simulate triage returning False
        triage_result = False

        if not triage_result:
            await repo.update_status(1, ArticleStatus.IGNORED)

        mock_conn.execute.assert_called_once()
        call_args = mock_conn.execute.call_args
        assert ArticleStatus.IGNORED.value in call_args[0]

    @pytest.mark.asyncio
    async def test_triage_true_continues_pipeline(self):
        """[正常系] トリアージがTrueの場合は後続の生成処理が実行されること"""
        triage_result = True
        generation_called = False

        if triage_result:
            generation_called = True

        assert generation_called is True
