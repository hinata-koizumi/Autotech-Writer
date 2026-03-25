"""Service for fact extraction and article generation."""

import logging
from typing import Optional

from app.config import Config
from app.models import ExtractionResult, ArticleResponse
from app.services.prompts import (
    EXTRACTION_SYSTEM_PROMPT,
    FULLTEXT_EXTRACTION_SYSTEM_PROMPT,
    THREAD_SYSTEM_PROMPT,
    NEWS_SYSTEM_PROMPT,
    X_TRANSLATE_SYSTEM_PROMPT,
)
from app.services.mocks import mock_extraction_response, mock_article_response
from .base import BaseLLMService

logger = logging.getLogger(__name__)


class ContentService(BaseLLMService):
    """Service for fact extraction and article generation."""

    def __init__(self, config: Config):
        super().__init__(config)
        self.provider = self._get_provider(config.llm.default_gen_provider)

    async def extract_facts(
        self,
        title: str,
        summary: str,
        source_type: str,
        source_url: str = "",
        full_content: Optional[str] = None,
    ) -> ExtractionResult:
        """Extract facts into strict JSON."""
        if self.config.dry_run and not self.provider:
            return mock_extraction_response()

        if not self.provider:
            raise ValueError("No LLM clients configured for generation.")

        # Determine which prompt to use based on content length
        is_fulltext = (
            full_content is not None
            and len(full_content) >= self.config.llm.fulltext_threshold
        )
        content_to_use = full_content if full_content else summary
        max_tokens = 4000

        if is_fulltext:
            logger.info(f"Using full-text extraction prompt for {title}")
            system_prompt = FULLTEXT_EXTRACTION_SYSTEM_PROMPT.format(
                full_text=content_to_use
            )
            max_tokens = 8000  # Increase for long papers
        else:
            system_prompt = EXTRACTION_SYSTEM_PROMPT.format(raw_text=content_to_use)

        user_prompt = (
            f"Source: {source_type}\nTitle: {title}\n"
            f"Source URL: {source_url}\n\n"
            "Respond ONLY with valid JSON."
        )

        parsed = await self._call_llm_json(
            self.provider, system_prompt, user_prompt, max_tokens=max_tokens
        )
        return ExtractionResult(**parsed)

    async def generate_article(self, extracted: ExtractionResult) -> ArticleResponse:
        """Generate the final markdown article from extracted facts."""
        if self.config.dry_run and not self.provider:
            return mock_article_response()

        if not self.provider:
            raise ValueError("No LLM clients configured for generation.")

        extracted_json = extracted.model_dump_json()

        if extracted.selected_format == "news":
            system_prompt = NEWS_SYSTEM_PROMPT.format(extracted_json=extracted_json)
        else:
            system_prompt = THREAD_SYSTEM_PROMPT.format(extracted_json=extracted_json)

        user_prompt = 'Generate the article based on the provided extracted facts. Respond ONLY with valid JSON in the format {"content": "..."}.'

        parsed = await self._call_llm_json(self.provider, system_prompt, user_prompt)
        response = ArticleResponse(**parsed)
        self._validate_article_response(response)
        return response

    def _validate_article_response(self, response: ArticleResponse) -> None:
        content = response.content.strip()
        if len(content) < self.config.min_article_length:
            raise ValueError(
                f"Generated article is too short ({len(content)} chars). Min required: {self.config.min_article_length}"
            )

        found_phrases = [p for p in self.config.ai_ng_phrases if p in content]
        if found_phrases:
            raise ValueError(
                f"Article contains forbidden AI-typical phrases: {found_phrases}"
            )

    async def translate_x_post(self, raw_text: str) -> ArticleResponse:
        """Translate an X post using the specialized prompt."""
        if self.config.dry_run and not self.provider:
            return ArticleResponse(
                content="[DRY-RUN] 翻訳済みテキスト: " + raw_text[:50] + "..."
            )

        if not self.provider:
            raise ValueError("No LLM clients configured for translation.")

        system_prompt = X_TRANSLATE_SYSTEM_PROMPT.format(raw_text=raw_text)
        user_prompt = (
            "Translate the post as instructed. Respond ONLY with the translated text."
        )

        try:
            translated_text = await self.provider.call(system_prompt, user_prompt)
            translated_text = translated_text.strip().strip('"').strip("'")
            return ArticleResponse(content=translated_text)
        except Exception as e:
            logger.error(f"Error during X post translation: {type(e).__name__}: {e}")
            raise
