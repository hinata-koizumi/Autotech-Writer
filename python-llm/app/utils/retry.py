"""Exponential backoff retry utilities for API rate limit handling."""

import asyncio
import logging
import random
from typing import TypeVar, Callable, Any, Type

logger = logging.getLogger(__name__)

T = TypeVar("T")

# Default retryable exceptions for LLM APIs
RETRYABLE_STATUS_CODES = {429, 500, 502, 503, 504}


def _is_retryable_exception(exc: Exception) -> bool:
    """Check if an exception is retryable (rate limit or transient server error)."""
    # OpenAI rate limit
    try:
        from openai import RateLimitError as OpenAIRateLimitError
        from openai import APIStatusError as OpenAIAPIStatusError

        if isinstance(exc, OpenAIRateLimitError):
            return True
        if isinstance(exc, OpenAIAPIStatusError) and exc.status_code in RETRYABLE_STATUS_CODES:
            return True
    except ImportError:
        pass

    # Anthropic rate limit
    try:
        from anthropic import RateLimitError as AnthropicRateLimitError
        from anthropic import APIStatusError as AnthropicAPIStatusError

        if isinstance(exc, AnthropicRateLimitError):
            return True
        if isinstance(exc, AnthropicAPIStatusError) and exc.status_code in RETRYABLE_STATUS_CODES:
            return True
    except ImportError:
        pass

    # httpx status errors (for PDF service, etc.)
    try:
        import httpx

        if isinstance(exc, httpx.HTTPStatusError) and exc.response.status_code in RETRYABLE_STATUS_CODES:
            return True
    except ImportError:
        pass

    return False


async def retry_with_backoff(
    func: Callable[..., Any],
    *args: Any,
    max_retries: int = 3,
    base_delay: float = 1.0,
    max_delay: float = 60.0,
    retryable_check: Callable[[Exception], bool] = _is_retryable_exception,
    **kwargs: Any,
) -> Any:
    """
    Execute an async function with exponential backoff retry on transient errors.

    Args:
        func: Async function to call.
        *args: Positional arguments for func.
        max_retries: Maximum number of retry attempts.
        base_delay: Initial delay in seconds before first retry.
        max_delay: Maximum delay cap in seconds.
        retryable_check: Function to determine if an exception is retryable.
        **kwargs: Keyword arguments for func.

    Returns:
        The return value of func.

    Raises:
        The last exception if all retries are exhausted.
    """
    last_exception: Exception | None = None

    for attempt in range(max_retries + 1):
        try:
            return await func(*args, **kwargs)
        except Exception as e:
            last_exception = e

            if attempt >= max_retries or not retryable_check(e):
                logger.error(
                    f"Non-retryable error or max retries ({max_retries}) exhausted: "
                    f"{type(e).__name__}: {e}"
                )
                raise

            # Exponential backoff with jitter
            jitter = random.uniform(0, base_delay * 0.5)
            delay = min(base_delay * (2**attempt) + jitter, max_delay)

            logger.warning(
                f"Retryable error (attempt {attempt + 1}/{max_retries}): "
                f"{type(e).__name__}: {e}. Retrying in {delay:.1f}s..."
            )
            await asyncio.sleep(delay)

    # Should not reach here, but just in case
    if last_exception:
        raise last_exception
