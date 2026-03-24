import logging
import re
from typing import Optional, Any
import asyncio
import tweepy
from app.config import Config
from app.models import ArticleResponse

logger = logging.getLogger(__name__)


class XApiService:
    """Service to post articles and threads to X using Tweepy."""

    def __init__(self, config: Config):
        self.config = config
        self.splitter = XTextSplitter()
        xc = self.config.x

        # Initialize Tweepy AsyncClient
        from tweepy.asynchronous import AsyncClient

        self.client = AsyncClient(
            consumer_key=xc.api_key,
            consumer_secret=xc.api_secret,
            access_token=xc.access_token,
            access_token_secret=xc.access_secret,
            wait_on_rate_limit=True,
        )

    async def post_tweet(
        self,
        text: str,
        *,
        reply_to: Optional[str] = None,
        media_ids: Optional[list[str]] = None,
    ) -> Optional[str]:
        """Post a single tweet optionally as a reply. Returns the tweet ID."""
        if self.config.dry_run:
            logger.info(
                f"DRY-RUN: Simulated tweet{' (reply_to=' + reply_to + ')' if reply_to else ''}"
            )
            logger.debug(f"DRY-RUN content: {text[:120]}...")
            import hashlib

            return f"dry_run_{hashlib.md5(text.encode()).hexdigest()[:8]}"

        xc = self.config.x
        if not all([xc.api_key, xc.api_secret, xc.access_token, xc.access_secret]):
            raise ValueError("X API credentials not configured")

        try:
            # Tweepy create_tweet arguments:
            # in_reply_to_tweet_id should be an int or str
            response = await self.client.create_tweet(
                text=text, in_reply_to_tweet_id=reply_to, media_ids=media_ids
            )

            tweet_id = str(response.data["id"])
            logger.debug(f"Successfully posted tweet: {tweet_id}")
            return tweet_id
        except tweepy.TweepyException as e:
            logger.error(f"Tweepy error posting to X: {e}")
            raise
        except Exception as e:
            logger.error(f"Unexpected error posting to X: {type(e).__name__}: {e}")
            raise

    async def post_article(
        self,
        article: ArticleResponse,
        *,
        media_ids: Optional[list[str]] = None,
        start_index: int = 0,
        last_id: Optional[str] = None,
        on_success_callback: Optional[Any] = None,
    ) -> list[str]:
        """
        Post a multi-tweet thread representing a long-form article.
        Supports resumption from start_index using last_id.
        """
        formatted_text = self._format_for_x(article.content)
        chunks = self.splitter.split(formatted_text)
        tweet_ids = []
        current_last_id = last_id

        # Adjust chunks based on start_index
        remaining_chunks = chunks[start_index:]

        for i, chunk in enumerate(remaining_chunks):
            real_index = start_index + i
            # Only attach media to the first tweet of the WHOLE thread (index 0)
            current_media = media_ids if real_index == 0 else None

            tweet_id = await self.post_tweet(
                chunk,
                reply_to=current_last_id,
                media_ids=current_media,
            )

            if tweet_id:
                tweet_ids.append(tweet_id)
                current_last_id = tweet_id

                # Report success back to caller for persistence
                if on_success_callback:
                    await on_success_callback(tweet_id, real_index)

            # Rate limit mitigation for threads
            if real_index < len(chunks) - 1:
                await asyncio.sleep(self.config.x.thread_interval_seconds)

        logger.info(f"Finished posting thread chunks from index {start_index}.")
        return tweet_ids

    def _format_for_x(self, text: str) -> str:
        """Convert Markdown to plain text suitable for X."""

        # 1. Replace Headers with emojis
        text = re.sub(r"^###\s+(.*)$", r"🔹 \1", text, flags=re.MULTILINE)
        text = re.sub(r"^##\s+(.*)$", r"📌 \1", text, flags=re.MULTILINE)
        text = re.sub(r"^#\s+(.*)$", r"📣 \1", text, flags=re.MULTILINE)

        # 2. Remove bold and italic markers
        text = re.sub(r"\*\*(.*?)\*\*", r"\1", text)
        text = re.sub(r"__(.*?)__", r"\1", text)
        text = re.sub(r"\*(.*?)\*", r"\1", text)
        text = re.sub(r"_(.*?)_", r"\1", text)

        # 3. Remove code block markers
        text = re.sub(r"```(?:[a-zA-Z0-9]+)?\n(.*?)\n```", r"\1", text, flags=re.DOTALL)
        text = re.sub(r"`(.*?)`", r"\1", text)

        # 4. Links: [text](http://...) -> text: http://...
        text = re.sub(r"\[(.*?)\]\((.*?)\)", r"\1: \2", text)

        return text.strip()

    async def close(self):
        """No explicit close needed for tweepy Client."""
        pass


class XTextSplitter:
    """Utility to split long text into X-compatible chunks (threads)."""

    def __init__(self, max_chars: int = 280):
        self.max_chars = max_chars

    def split(self, text: str) -> list[str]:
        """Split article into chunks based on '---' separator or paragraphs if needed."""
        # 1. First split by '---' which is our primary separator from LLM
        initial_chunks = [c.strip() for c in text.split("---") if c.strip()]

        # 2. If we still have chunks exceeding max_chars, split them further
        raw_chunks = []
        for chunk in initial_chunks:
            if len(chunk) <= self.max_chars:
                raw_chunks.append(chunk)
            else:
                raw_chunks.extend(self._split_long_chunk(chunk))

        return self._prepare_final_chunks(raw_chunks)

    def _split_long_chunk(self, chunk: str) -> list[str]:
        """Split a single chunk that exceeds max_chars into smaller pieces."""
        paras = chunk.split("\n\n")
        sub_chunks = []
        current = ""

        for p in paras:
            p_parts = self._split_paragraph_if_needed(p)
            for part in p_parts:
                if not current:
                    current = part
                elif len(current) + 2 + len(part) <= self.max_chars:
                    current += "\n\n" + part
                else:
                    sub_chunks.append(current)
                    current = part
        if current:
            sub_chunks.append(current)
        return sub_chunks

    def _split_paragraph_if_needed(self, p: str) -> list[str]:
        """Split a single paragraph if it exceeds max_chars."""
        if len(p) <= self.max_chars:
            return [p]

        # Split by space
        words = p.split(" ")
        sub_paras = []
        current = ""
        for w in words:
            if not current:
                current = w
            elif len(current) + 1 + len(w) <= self.max_chars:
                current += " " + w
            else:
                sub_paras.append(current)
                current = w
        if current:
            sub_paras.append(current)

        # Character-level split if still too long
        return self._split_by_characters(sub_paras)

    def _split_by_characters(self, sub_paras: list[str]) -> list[str]:
        """Fallback to character-level split for very long tokens."""
        char_limit = self.max_chars - 10
        final_parts = []
        for sp in sub_paras:
            if len(sp) > self.max_chars:
                while sp:
                    final_parts.append(sp[:char_limit])
                    sp = sp[char_limit:]
            else:
                final_parts.append(sp)
        return final_parts

    def _prepare_final_chunks(self, raw_chunks: list[str]) -> list[str]:
        """Clean up chunks and add thread indices (e.g., 1/N)."""
        final_chunks = []
        total = len(raw_chunks)
        for i, chunk in enumerate(raw_chunks):
            img_pattern: str = r"\[IMAGE:\s*.*?\]"
            cleaned_chunk: str = re.sub(img_pattern, "", chunk).strip()

            # Add thread index (1/N) if there are more than 1 chunks
            if total > 1:
                index_str = f" ({i+1}/{total})"
                # Only truncate if the content itself exceeds max_chars
                if len(cleaned_chunk) > self.max_chars:
                    limit = self.max_chars - len(index_str) - 3
                    cleaned_chunk = cleaned_chunk[:limit] + "..."
                cleaned_chunk = cleaned_chunk + index_str
            else:
                if len(cleaned_chunk) > self.max_chars:
                    cleaned_chunk = cleaned_chunk[: self.max_chars - 3] + "..."

            if cleaned_chunk:
                final_chunks.append(cleaned_chunk)

        return final_chunks
