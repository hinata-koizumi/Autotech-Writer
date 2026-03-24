"""Pipeline logic connecting DB repository, LLM triage/generation, and X API."""

import json
import logging
from typing import Dict, Any, Optional

from app.repository import ArticleRepository
from app.services.llm import LLMService
from app.services.x_api import XApiService
from app.services.compliance import check_compliance
from app.config import Config
from app.models import ExtractionResult, ArticleResponse, ArticleStatus, ArticleUpdate

logger = logging.getLogger(__name__)


async def process_article(
    article: Dict[str, Any],
    repo: ArticleRepository,
    llm_service: LLMService,
    x_api: XApiService,
    config: Config,
    pdf_service: Optional[any] = None,
) -> None:
    """Process a single article through the full pipeline."""
    article_id = article["id"]
    logger.info(
        f"Processing article {article_id}: {article['title']} (Status: {article.get('status')})"
    )

    try:
        if _is_resuming(article):
            article_response = _resume_from_partial_failure(article)
        else:
            article_response, hook_text = await _prepare_new_content(
                article, repo, llm_service, config, pdf_service=pdf_service
            )
            if article_response is None:
                return  # Triaged or rejected

            await _mark_as_posting(article_id, article_response, hook_text, repo)

            if config.stay_as_draft:
                await _save_as_draft(article_id, repo)
                return

        await _posting_step(article, article_response, repo, x_api)

    except Exception as e:
        logger.error(
            f"Error processing article {article_id}: {type(e).__name__}: {e}",
            exc_info=True,
        )
        await _handle_processing_error(article, repo, config)
        raise


def _is_resuming(article: Dict[str, Any]) -> bool:
    return article.get("status") == ArticleStatus.PARTIAL_FAILED.value


def _resume_from_partial_failure(article: Dict[str, Any]) -> ArticleResponse:
    logger.info(
        f"Resuming article {article['id']} from {ArticleStatus.PARTIAL_FAILED.value}"
    )
    return ArticleResponse(content=article["article_body"])


async def _mark_as_posting(
    article_id: int, response: ArticleResponse, hook_text: str, repo: ArticleRepository
):
    await repo.update_status(
        article_id,
        ArticleUpdate(
            status=ArticleStatus.POSTING,
            hook_text=hook_text,
            article_body=response.content,
        ),
    )


async def _save_as_draft(article_id: int, repo: ArticleRepository):
    logger.info(f"Article {article_id} saved as DRAFT.")
    await repo.update_status(article_id, ArticleUpdate(status=ArticleStatus.DRAFT))


async def _prepare_new_content(
    article: Dict[str, Any],
    repo: ArticleRepository,
    llm_service: LLMService,
    config: Config,
    pdf_service: Optional[any] = None,
) -> tuple[Optional[ArticleResponse], str]:
    """Triage, extract, and generate article content."""
    article_id = article["id"]

    if article["source_type"] == "x_post":
        return await _handle_x_post_translation(article, llm_service)

    # ArXiv PDF Fallback
    if (
        article["source_type"] == "arxiv"
        and not article.get("full_content")
        and pdf_service
    ):
        metadata = (
            json.loads(article.get("metadata", "{}"))
            if isinstance(article.get("metadata"), str)
            else article.get("metadata", {})
        )
        pdf_url = metadata.get("pdf_url")
        if pdf_url:
            pdf_text = await pdf_service.extract_text_from_url(pdf_url)
            if pdf_text:
                logger.info(f"Using extracted PDF text for article {article_id}")
                article["full_content"] = pdf_text

    # 1. Triage
    if not await _triage_step(article, llm_service, repo):
        return None, ""

    # 2. Extraction & Compliance
    extracted_facts = await _extraction_step(article, repo, llm_service, config)
    if not extracted_facts:
        return None, ""

    # 3. Generation
    return await _generation_step(article_id, extracted_facts, llm_service)


async def _handle_x_post_translation(
    article: Dict[str, Any], llm_service: LLMService
) -> tuple[ArticleResponse, str]:
    logger.info(f"Article {article['id']} is an X post. Translating...")
    article_response = await llm_service.translate_x_post(article.get("summary", ""))
    return article_response, article["title"]


async def _triage_step(
    article: Dict[str, Any], llm_service: LLMService, repo: ArticleRepository
) -> bool:
    is_valuable = await llm_service.triage_article(
        article["title"], article.get("summary", ""), article["source_type"]
    )
    if not is_valuable:
        logger.info(f"Article {article['id']} triaged as not valuable.")
        await repo.update_status(
            article["id"], ArticleUpdate(status=ArticleStatus.IGNORED)
        )
        return False
    return True


async def _extraction_step(
    article: Dict[str, Any],
    repo: ArticleRepository,
    llm_service: LLMService,
    config: Config,
) -> Optional[ExtractionResult]:
    article_id = article["id"]
    full_content = await repo.get_full_content(article_id)
    extracted_facts = await llm_service.extract_facts(
        article["title"],
        article.get("summary", ""),
        article["source_type"],
        source_url=article.get("url", ""),
        full_content=full_content,
    )

    if not _is_extraction_valid(article_id, extracted_facts, config):
        await repo.update_status(
            article_id, ArticleUpdate(status=ArticleStatus.IGNORED)
        )
        return None

    if not await _check_article_compliance(article_id, extracted_facts, repo, config):
        return None

    return extracted_facts


async def _generation_step(
    article_id: int, extracted_facts: ExtractionResult, llm_service: LLMService
) -> tuple[ArticleResponse, str]:
    logger.info(f"Article {article_id} facts extracted. Generating article...")
    article_response = await llm_service.generate_article(extracted_facts)
    hook_text = (
        extracted_facts.problem_statement[:200]
        if extracted_facts.problem_statement
        else ""
    )
    return article_response, hook_text


def _is_extraction_valid(
    article_id: int, extracted: ExtractionResult, config: Config
) -> bool:
    """Check if extracted facts meet quality bars."""
    if not extracted.is_information_sufficient:
        logger.info(
            f"Article {article_id} ignored due to insufficient information. Reason: {extracted.reason_for_insufficient}"
        )
        return False

    if extracted.extraction_confidence < 0.7:
        logger.info(
            f"Article {article_id} ignored due to low extraction confidence ({extracted.extraction_confidence})."
        )
        return False

    return True


async def _check_article_compliance(
    article_id: int,
    extracted: ExtractionResult,
    repo: ArticleRepository,
    config: Config,
) -> bool:
    """Check if extracted content complies with safety/NG word rules."""
    extraction_text = extracted.model_dump_json()
    is_clean, detected_words = check_compliance(extraction_text, config.ng_keywords)
    if not is_clean:
        logger.warning(
            f"Article {article_id} rejected after extraction due to compliance. Words: {detected_words}"
        )
        await repo.update_status(
            article_id, ArticleUpdate(status=ArticleStatus.REJECTED_COMPLIANCE)
        )
        return False
    return True


def _resolve_posting_params(
    article: Dict[str, Any],
) -> tuple[list[str], Optional[str], int]:
    """Determine initial thread IDs, last ID to reply to, and start index for posting."""
    is_resuming = article.get("status") == ArticleStatus.PARTIAL_FAILED.value

    current_thread_ids = []
    if is_resuming and article.get("x_thread_ids"):
        current_thread_ids = json.loads(article["x_thread_ids"])

    last_id = current_thread_ids[-1] if current_thread_ids else None
    start_idx = article.get("last_posted_index", 0)
    if is_resuming and current_thread_ids:
        start_idx += 1

    return current_thread_ids, last_id, start_idx


async def _posting_step(
    article: Dict[str, Any],
    article_response: ArticleResponse,
    repo: ArticleRepository,
    x_api: XApiService,
) -> None:
    """Handle the actual posting to X API."""
    article_id = article["id"]
    current_thread_ids, last_id, start_idx = _resolve_posting_params(article)

    async def _on_success(tweet_id: str, index: int):
        current_thread_ids.append(tweet_id)
        await repo.update_status(
            article_id,
            ArticleUpdate(
                status=ArticleStatus.POSTING,
                x_post_id=current_thread_ids,
                last_posted_index=index,
            ),
        )

    try:
        await x_api.post_article(
            article_response,
            start_index=start_idx,
            last_id=last_id,
            on_success_callback=_on_success,
        )
        await repo.update_status(
            article_id, ArticleUpdate(status=ArticleStatus.COMPLETED)
        )
        logger.info(
            f"Successfully processed and posted article {article_id} as thread."
        )
    except Exception as e:
        logger.error(f"X API error during posting for article {article_id}: {e}")
        await repo.update_status(
            article_id, ArticleUpdate(status=ArticleStatus.PARTIAL_FAILED)
        )
        raise


async def _handle_processing_error(
    article: Dict[str, Any], repo: ArticleRepository, config: Config
) -> None:
    """Handle errors by incrementing retry count if appropriate."""
    article_id = article["id"]
    status = article.get("status")
    if status not in [ArticleStatus.PARTIAL_FAILED.value, ArticleStatus.POSTING.value]:
        try:
            new_status = await repo.increment_retry(
                article_id, max_retries=config.max_retries
            )
            logger.info(
                f"Article {article_id} marked as {new_status.value} after error."
            )
        except Exception as retry_err:
            logger.error(
                f"Failed to increment retry for article {article_id}: {retry_err}"
            )
