"""Tests for X API service — thread posting with mock HTTP."""

import asyncio
from unittest.mock import AsyncMock, patch
import pytest
import hashlib

from app.config import Config
from app.models import ArticleResponse
from app.services.x_api import XApiService


# ===================================================================
# Helpers
# ===================================================================

def _make_dry_run_config(**overrides) -> Config:
    cfg = Config(dry_run=True)
    cfg.x.thread_interval_seconds = overrides.get("thread_interval_seconds", 0.0)
    return cfg


def _make_article(content: str) -> ArticleResponse:
    # Bypass validation for testing purposes by padding to required length
    if len(content) < 1000:
        content = content + "\n\n" + ("X" * (1000 - len(content)))
    return ArticleResponse(content=content)


# ===================================================================
# Tests
# ===================================================================


class TestPostTweet:
    """Tests for single tweet posting."""

    @pytest.mark.asyncio
    async def test_dry_run_returns_id(self):
        """[正常系] dry-run モードでツイートIDが返ること"""
        config = _make_dry_run_config()
        svc = XApiService(config)
        tweet_id = await svc.post_tweet("hello")
        assert tweet_id is not None
        assert tweet_id.startswith("dry_run_")

    @pytest.mark.asyncio
    async def test_dry_run_reply_to(self):
        """[正常系] dry-run モードで reply_to が指定できること"""
        config = _make_dry_run_config()
        svc = XApiService(config)
        tweet_id = await svc.post_tweet("reply", reply_to="12345")
        assert tweet_id is not None

    @pytest.mark.asyncio
    async def test_no_token_raises(self):
        """[異常系] dry-run=False でトークン無しの場合はエラー"""
        config = Config(dry_run=False)
        config.x.access_token = ""
        with pytest.raises(ValueError, match="X API credentials not configured"):
            svc = XApiService(config)
            await svc.post_tweet("hello")


class TestPostArticleThread:
    """Tests for multi-post thread posting via post_article."""

    @pytest.mark.asyncio
    async def test_post_article_splits_and_returns_ids(self):
        """[正常系] 長い記事が分割され、複数のツイートIDが返ること"""
        config = _make_dry_run_config(thread_interval_seconds=0.0)
        svc = XApiService(config)
        
        # Create a long article content (> 260 chars)
        content = "Line 1\n\n" + ("A" * 200) + "\n\nLine 2\n\n" + ("B" * 200)
        article = _make_article(content)
        
        ids = await svc.post_article(article)
        assert len(ids) >= 2
        assert all(isinstance(tid, str) for tid in ids)

    @pytest.mark.asyncio
    async def test_article_chaining_order(self):
        """[正常系] 各ポストが前のIDを reply_to に指定してチェーンされること"""
        config = _make_dry_run_config(thread_interval_seconds=0.0)
        svc = XApiService(config)

        call_args = []
        
        async def tracking_post(text, *, reply_to=None, media_ids=None):
            call_args.append({"text": text, "reply_to": reply_to})
            return f"id_{len(call_args)}"

        with patch.object(svc, "post_tweet", side_effect=tracking_post):
            content = "Chunk 1\n\n" + ("X" * 300) + "\n\nChunk 2"
            article = _make_article(content)
            ids = await svc.post_article(article)

        assert len(ids) > 1
        # First post has no reply_to
        assert call_args[0]["reply_to"] is None
        # Subsequent posts reply to the previous tweet
        for i in range(1, len(ids)):
            assert call_args[i]["reply_to"] == ids[i - 1]

    @pytest.mark.asyncio
    async def test_article_interval_applied(self):
        """[正常系] thread_interval_seconds が適用されること"""
        config = _make_dry_run_config(thread_interval_seconds=1.5)
        svc = XApiService(config)

        sleep_calls = []
        async def mock_sleep(seconds):
            sleep_calls.append(seconds)

        # Content that will be split in 2
        content = "A" * 200 + "\n\n" + "B" * 200
        article = _make_article(content)

        with patch("asyncio.sleep", side_effect=mock_sleep):
            await svc.post_article(article)
    
        # Padding increases the number of chunks to 5, so sleep is called 4 times.
        assert len(sleep_calls) == 4
        assert sleep_calls[0] == 1.5

    def test_split_article_logic(self):
        """[正常系] 記事の分割ロジックが期待通り動作すること"""
        from app.services.x_api import XTextSplitter
        splitter = XTextSplitter(max_chars=15)
        
        text = "Hello World\n\n" + "Second para"
        chunks = splitter.split(text)
        
        assert len(chunks) == 2
        assert "Hello World" in chunks[0]
        assert "Second para" in chunks[1]
        assert "(1/2)" in chunks[0]
        assert "(2/2)" in chunks[1]

    def test_format_for_x_strips_markdown(self):
        """[正常系] MarkdownフォーマットがX用のプレーンテキストに正しく変換されること"""
        config = _make_dry_run_config()
        svc = XApiService(config)
        
        md_text = (
            "# Main Title\n\n"
            "This is **bold** and __also bold__.\n"
            "And *italic* and _also italic_.\n\n"
            "## Subtitle\n\n"
            "Here is a [link](https://example.com).\n\n"
            "### Section\n\n"
            "```python\nprint('hello')\n```\n"
            "Inline `code` test."
        )
        
        expected = (
            "📣 Main Title\n\n"
            "This is bold and also bold.\n"
            "And italic and also italic.\n\n"
            "📌 Subtitle\n\n"
            "Here is a link: https://example.com.\n\n"
            "🔹 Section\n\n"
            "print('hello')\n"
            "Inline code test."
        )
        
        formatted = svc._format_for_x(md_text)
        assert formatted == expected
