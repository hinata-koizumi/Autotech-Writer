"""Tests for exponential backoff retry utility."""

import asyncio
import pytest
from unittest.mock import AsyncMock, patch

from app.utils.retry import retry_with_backoff, _is_retryable_exception


# --- Test _is_retryable_exception ---


class TestIsRetryableException:
    def test_generic_exception_not_retryable(self):
        assert _is_retryable_exception(ValueError("test")) is False

    def test_openai_rate_limit_is_retryable(self):
        try:
            from openai import RateLimitError

            # RateLimitError requires specific args in newer SDKs
            exc = RateLimitError(
                message="Rate limit exceeded",
                response=AsyncMock(status_code=429),
                body=None,
            )
            assert _is_retryable_exception(exc) is True
        except (ImportError, TypeError):
            pytest.skip("openai SDK not available or incompatible")

    def test_anthropic_rate_limit_is_retryable(self):
        try:
            from anthropic import RateLimitError

            exc = RateLimitError(
                message="Rate limit exceeded",
                response=AsyncMock(status_code=429),
                body=None,
            )
            assert _is_retryable_exception(exc) is True
        except (ImportError, TypeError):
            pytest.skip("anthropic SDK not available or incompatible")


# --- Test retry_with_backoff ---


class TestRetryWithBackoff:
    @pytest.mark.asyncio
    async def test_success_on_first_try(self):
        """正常系: 1回目で成功する場合"""
        mock_func = AsyncMock(return_value="success")
        result = await retry_with_backoff(mock_func, max_retries=3, base_delay=0.01)
        assert result == "success"
        assert mock_func.call_count == 1

    @pytest.mark.asyncio
    async def test_retry_on_retryable_error(self):
        """異常系→正常系: リトライ可能エラーの後に成功"""
        call_count = 0

        async def flaky_func():
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise ConnectionError("transient error")
            return "success"

        # ConnectionError is not retryable by default
        # Use a custom retryable_check that allows it
        result = await retry_with_backoff(
            flaky_func,
            max_retries=3,
            base_delay=0.01,
            retryable_check=lambda e: isinstance(e, ConnectionError),
        )
        assert result == "success"
        assert call_count == 3

    @pytest.mark.asyncio
    async def test_exhausted_retries_raises(self):
        """異常系: 最大リトライ回数超過でエラーが発生"""
        mock_func = AsyncMock(side_effect=ConnectionError("always fails"))

        with pytest.raises(ConnectionError, match="always fails"):
            await retry_with_backoff(
                mock_func,
                max_retries=2,
                base_delay=0.01,
                retryable_check=lambda e: isinstance(e, ConnectionError),
            )
        # 1 initial + 2 retries = 3 total calls
        assert mock_func.call_count == 3

    @pytest.mark.asyncio
    async def test_non_retryable_error_raises_immediately(self):
        """異常系: リトライ不可のエラーは即時発生"""
        mock_func = AsyncMock(side_effect=ValueError("not retryable"))

        with pytest.raises(ValueError, match="not retryable"):
            await retry_with_backoff(
                mock_func,
                max_retries=3,
                base_delay=0.01,
                retryable_check=lambda e: isinstance(e, ConnectionError),
            )
        # Should only try once since ValueError is not retryable
        assert mock_func.call_count == 1

    @pytest.mark.asyncio
    async def test_delay_increases_exponentially(self):
        """バックオフ遅延が指数関数的に増加することを確認"""
        delays = []

        original_sleep = asyncio.sleep

        async def mock_sleep(delay):
            delays.append(delay)
            # Don't actually sleep in tests

        mock_func = AsyncMock(side_effect=ConnectionError("always fails"))

        with patch("app.utils.retry.asyncio.sleep", side_effect=mock_sleep):
            with pytest.raises(ConnectionError):
                await retry_with_backoff(
                    mock_func,
                    max_retries=3,
                    base_delay=1.0,
                    max_delay=60.0,
                    retryable_check=lambda e: isinstance(e, ConnectionError),
                )

        # Should have 3 delays (one before each retry)
        assert len(delays) == 3
        # Each delay should be larger than or equal to the previous (accounting for jitter)
        # base_delay * 2^0 + jitter, base_delay * 2^1 + jitter, base_delay * 2^2 + jitter
        assert delays[0] >= 1.0  # 2^0 = 1
        assert delays[1] >= 2.0  # 2^1 = 2
        assert delays[2] >= 4.0  # 2^2 = 4

    @pytest.mark.asyncio
    async def test_max_delay_cap(self):
        """最大遅延がキャップされることを確認"""
        delays = []

        async def mock_sleep(delay):
            delays.append(delay)

        mock_func = AsyncMock(side_effect=ConnectionError("always fails"))

        with patch("app.utils.retry.asyncio.sleep", side_effect=mock_sleep):
            with pytest.raises(ConnectionError):
                await retry_with_backoff(
                    mock_func,
                    max_retries=5,
                    base_delay=10.0,
                    max_delay=30.0,
                    retryable_check=lambda e: isinstance(e, ConnectionError),
                )

        # All delays should be <= max_delay + jitter
        for delay in delays:
            assert delay <= 35.0  # max_delay (30) + max jitter (10 * 0.5 = 5)
