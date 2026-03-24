"""LLM service for article triage and generation."""

from app.config import Config
from .triage import TriageService
from .content import ContentService


class LLMService:
    """Facade for LLM services."""

    def __init__(self, config: Config):
        self.triage_service = TriageService(config)
        self.content_service = ContentService(config)

    async def triage_article(self, *args, **kwargs):
        return await self.triage_service.triage_article(*args, **kwargs)

    async def extract_facts(self, *args, **kwargs):
        return await self.content_service.extract_facts(*args, **kwargs)

    async def generate_article(self, *args, **kwargs):
        return await self.content_service.generate_article(*args, **kwargs)

    async def translate_x_post(self, *args, **kwargs):
        return await self.content_service.translate_x_post(*args, **kwargs)
