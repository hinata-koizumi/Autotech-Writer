"""Service to determine if content is valuable."""

import logging
from app.config import Config
from app.services.prompts import TRIAGE_SYSTEM_PROMPT
from .base import BaseLLMService

logger = logging.getLogger(__name__)


class TriageService(BaseLLMService):
    """Service to determine if content is valuable."""

    def __init__(self, config: Config):
        super().__init__(config)
        self.provider = self._get_provider(config.llm.default_triage_provider)

    async def triage_article(self, title: str, summary: str, source_type: str) -> bool:
        if self.config.dry_run and not self.provider:
            return True

        if not self.provider:
            raise ValueError("No LLM clients configured for triage.")

        prompt = TRIAGE_SYSTEM_PROMPT.format(
            source_type=source_type, title=title, summary=summary
        )

        try:
            # Triage usually uses empty system prompt or specialized one
            parsed = await self._call_llm_json(
                self.provider, 
                system_prompt="", 
                user_prompt=prompt,
                temperature=0.0,
                max_tokens=200,
                is_triage=True
            )
            val = parsed.get("value", False)
            reason = parsed.get("reason", "No reason provided")
            
            if not val:
                logger.info(f"Article triaged as false. Reason: {reason}")
            
            return bool(val)
        except Exception as e:
            logger.error(f"Error during triage: {type(e).__name__}: {e}")
            return False
