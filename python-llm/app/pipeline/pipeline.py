"""Pipeline logic connecting DB repository, LLM triage/generation, and X API."""

import logging
from typing import Dict, Any

from app.repository import ArticleRepository, ArticleStatus
from app.services.llm import LLMService
from app.services.x_api import XApiService
from app.services.compliance import check_compliance
from app.config import Config

logger = logging.getLogger(__name__)


async def process_article(
    article: Dict[str, Any],
    repo: ArticleRepository,
    llm_service: LLMService,
    x_api: XApiService,
    config: Config,
) -> None:
    """
    Process a single pending article through the full pipeline:
    1. Mark as processing
    2. Compliance check
    3. LLM Triage
    4. LLM Generation
    5. X Posting
    6. Mark as completed
    """
    article_id = article["id"]
    title = article["title"]
    summary = article.get("summary", "")
    source_type = article["source_type"]
    
    logger.info(f"Processing article {article_id}: {title}")

    try:
        # 1. Update status to PROCESSING
        await repo.update_status(article_id, ArticleStatus.PROCESSING)

        # 2. Compliance check (NG keywords)
        text_to_check = f"{title}\n{summary}"
        is_clean, detected_words = check_compliance(text_to_check)
        if not is_clean:
            logger.warning(f"Article {article_id} rejected due to compliance. Words: {detected_words}")
            await repo.update_status(article_id, ArticleStatus.REJECTED_COMPLIANCE)
            return

        # 3. LLM Triage
        is_valuable = await llm_service.triage_article(title, summary, source_type)
        if not is_valuable:
            logger.info(f"Article {article_id} triaged as not valuable.")
            await repo.update_status(article_id, ArticleStatus.IGNORED)
            return
            
        logger.info(f"Article {article_id} passed triage. Generating content...")

        # 4. LLM Generation
        generated_text = await llm_service.generate_article(title, summary, source_type)
        if not generated_text:
            raise ValueError("LLM generated empty text.")

        if len(generated_text) < config.min_article_length and not config.dry_run:
            logger.warning(f"Generated text too short ({len(generated_text)} chars).")

        # 5. X Posting
        logger.info(f"Article {article_id} generated length: {len(generated_text)}. Posting to X...")
        x_post_id = await x_api.post_article(generated_text)

        # 6. Update Status to COMPLETED
        await repo.update_status(
            article_id, 
            ArticleStatus.COMPLETED,
            article_body=generated_text,
            x_post_id=x_post_id
        )
        logger.info(f"Successfully processed and posted article {article_id}.")

    except Exception as e:
        logger.error(f"Error processing article {article_id}: {e}", exc_info=True)
        try:
            # Handle failure / retry logic
            new_status = await repo.increment_retry(article_id, max_retries=config.max_retries)
            logger.info(f"Article {article_id} marked as {new_status.value} after error.")
        except Exception as retry_err:
            logger.error(f"Failed to increment retry for article {article_id}: {retry_err}")
