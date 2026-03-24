"""Base classes and utility for LLM services."""

import json
import logging
from typing import Dict, Any, Optional

from app.config import Config
from app.utils.json_utils import extract_json
from .providers import LLMProvider, LLMProviderFactory

logger = logging.getLogger(__name__)


class BaseLLMService:
    """Base class for LLM-based services."""

    def __init__(self, config: Config):
        self.config = config
        self.providers: Dict[str, LLMProvider] = LLMProviderFactory.create_providers(
            config
        )

    def _get_provider(self, name: str) -> Optional[LLMProvider]:
        provider = self.providers.get(name)
        if not provider and self.providers:
            # Fallback to first available provider
            provider = next(iter(self.providers.values()))
        return provider

    async def _call_llm_json(
        self,
        provider: LLMProvider,
        system_prompt: str,
        user_prompt: str,
        temperature: float = 0.5,
        max_tokens: int = 4000,
        is_triage: bool = False,
    ) -> Dict[str, Any]:
        """Call LLM and extract JSON from the response."""
        try:
            raw_text = await provider.call(
                system_prompt=system_prompt,
                user_prompt=user_prompt,
                temperature=temperature,
                max_tokens=max_tokens,
                is_triage=is_triage,
            )

            logger.debug(f"Raw LLM response: {raw_text[:500]}...")
            json_text = extract_json(raw_text)
            return json.loads(json_text)
        except json.JSONDecodeError as e:
            logger.error(f"LLM returned invalid JSON: {raw_text[:200]}...")
            raise ValueError(f"LLM output is not valid JSON: {e}") from e
        except Exception as e:
            logger.error(
                f"Error during LLM call or JSON extraction: {type(e).__name__}: {e}"
            )
            raise
