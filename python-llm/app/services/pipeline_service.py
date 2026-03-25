import asyncio
import logging
from typing import List, Optional

from app.config import Config
from app.repository import ArticleRepository
from app.services.llm import LLMService
from app.services.x_api import XApiService
from app.services.line_notifier import LineNotifierService
from app.services.pdf_service import PDFService
from app.pipeline.pipeline import process_article

logger = logging.getLogger(__name__)


class PipelineService:
    def __init__(
        self,
        repo: ArticleRepository,
        llm_service: LLMService,
        x_api: XApiService,
        config: Config,
        line_notifier: Optional[LineNotifierService] = None,
    ):
        self.repo = repo
        self.llm_service = llm_service
        self.x_api = x_api
        self.config = config
        self.line_notifier = line_notifier
        self.pdf_service = PDFService()

    async def run_pipeline(self, limit: int = 10):
        """Fetch and process all pending articles."""
        try:
            articles = await self.repo.get_pending_articles(limit=limit)
            logger.info(f"Found {len(articles)} pending articles.")

            for article in articles:
                try:
                    await process_article(
                        article,
                        self.repo,
                        self.llm_service,
                        self.x_api,
                        self.config,
                        pdf_service=self.pdf_service,
                        line_notifier=self.line_notifier,
                    )
                except Exception as e:
                    logger.error(f"Failed to process article {article['id']}: {e}")

                await asyncio.sleep(self.config.article_interval_seconds)

        except Exception as e:
            logger.error(f"Error in run_pipeline: {e}")
            raise
