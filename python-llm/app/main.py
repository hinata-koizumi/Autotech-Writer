"""FastAPI application for Autotech Writer LLM Module."""

import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, BackgroundTasks, HTTPException, Depends, Request
import asyncpg
from typing import Annotated, Optional

from app.config import Config
from app.repository import ArticleRepository
from app.services.llm import LLMService
from app.services.x_api import XApiService
from app.services.pipeline_service import PipelineService
from app.services.pg_listener import PgListenerService
from app.services.line_notifier import LineNotifierService
from app.models import TriggerResponse, ArticleStatus, ArticleUpdate

from linebot.v3.webhook import WebhookParser
from linebot.v3.exceptions import InvalidSignatureError
from linebot.v3.webhooks import PostbackEvent

logging.basicConfig(
    level=logging.DEBUG, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# Global configuration
config = Config()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifecycle events for FastAPI app."""
    logger.info("Starting Autotech Writer LLM API...")

    # 1. Initialize Config
    app.state.config = config

    # 2. Initialize DB pool
    try:
        app.state.db_pool = await asyncpg.create_pool(
            dsn=config.db.url,
            min_size=config.db.pool_min_size,
            max_size=config.db.pool_max_size,
        )
    except Exception as e:
        logger.error(f"Failed to connect to database: {e}")
        raise RuntimeError(f"Database connection failed: {e}") from e

    # 3. Initialize Services (Singletons)
    app.state.llm_service = LLMService(config)
    app.state.x_api = XApiService(config)
    app.state.line_notifier = LineNotifierService(config)

    # 4. Initialize PG Listener
    async def _on_notification(_data: dict):
        """Callback triggered by PG notifications."""
        logger.info("PG Notification received: triggering pipeline...")
        # Create a transient PipelineService to run the task
        repo = ArticleRepository(app.state.db_pool)
        pipeline = PipelineService(
            repo,
            app.state.llm_service,
            app.state.x_api,
            config,
            line_notifier=app.state.line_notifier,
        )
        # Run pipeline in background to not block listener
        asyncio.create_task(pipeline.run_pipeline())

    app.state.pg_listener = PgListenerService(config, _on_notification)
    app.state.pg_listener.start()

    yield

    # Shutdown
    logger.info("Shutting down Autotech Writer LLM API...")
    if hasattr(app.state, "pg_listener"):
        await app.state.pg_listener.stop()
    if hasattr(app.state, "x_api"):
        await app.state.x_api.close()
    if hasattr(app.state, "db_pool"):
        await app.state.db_pool.close()


app = FastAPI(title="Autotech Writer LLM API", lifespan=lifespan)


# --- Dependencies ---


def get_config(request: Request) -> Config:
    return request.app.state.config


def get_db_pool(request: Request) -> asyncpg.Pool:
    return request.app.state.db_pool


def get_repo(pool: Annotated[asyncpg.Pool, Depends(get_db_pool)]) -> ArticleRepository:
    return ArticleRepository(pool)


def get_llm_service(request: Request) -> LLMService:
    return request.app.state.llm_service


def get_x_api(request: Request) -> XApiService:
    return request.app.state.x_api


def get_line_notifier(request: Request) -> LineNotifierService:
    return request.app.state.line_notifier


def get_pipeline_service(
    repo: Annotated[ArticleRepository, Depends(get_repo)],
    llm: Annotated[LLMService, Depends(get_llm_service)],
    x_api: Annotated[XApiService, Depends(get_x_api)],
    cfg: Annotated[Config, Depends(get_config)],
    line_notifier: Annotated[LineNotifierService, Depends(get_line_notifier)],
) -> PipelineService:
    return PipelineService(repo, llm, x_api, cfg, line_notifier=line_notifier)


# --- Background Tasks ---


async def run_pipeline_task(service: PipelineService):
    """Background task wrapper for PipelineService."""
    try:
        await service.run_pipeline()
    except Exception as e:
        logger.error(f"Error in run_pipeline_task: {e}")


@app.post("/trigger", response_model=TriggerResponse)
async def trigger_pipeline(
    background_tasks: BackgroundTasks,
    pipeline_service: Annotated[PipelineService, Depends(get_pipeline_service)],
):
    """
    Endpoint triggered by the Go collector module or chron.
    Starts processing of pending articles in the background.
    """
    logger.info("Received request to trigger pipeline.")
    background_tasks.add_task(run_pipeline_task, pipeline_service)
    return TriggerResponse(status="Processing started.")


@app.post("/line/webhook")
async def line_webhook(
    request: Request,
    background_tasks: BackgroundTasks,
    pipeline_service: Annotated[PipelineService, Depends(get_pipeline_service)],
    repo: Annotated[ArticleRepository, Depends(get_repo)],
):
    """
    LINE Messaging API Webhook.
    Handles user Approve/Reject postback actions.
    """
    signature = request.headers.get("X-Line-Signature", "")
    body = await request.body()
    body_text = body.decode("utf-8")

    parser = WebhookParser(config.line.channel_secret)

    try:
        events = parser.parse(body_text, signature)
    except InvalidSignatureError:
        logger.error("Invalid LINE signature.")
        raise HTTPException(status_code=400, detail="Invalid signature")

    for event in events:
        if isinstance(event, PostbackEvent):
            data_str = event.postback.data
            logger.info(f"Received LINE postback: {data_str}")

            # Parse simple 'key=value&key=value' format
            params = dict(p.split("=") for p in data_str.split("&") if "=" in p)
            action = params.get("action")
            article_id_str = params.get("article_id")

            if not action or not article_id_str:
                continue

            article_id = int(article_id_str)

            if action == "approve":
                logger.info(f"Article {article_id} approved via LINE.")
                await repo.update_status(
                    article_id, ArticleUpdate(status=ArticleStatus.APPROVED)
                )
                # Trigger pipeline to pick up the APPROVED article immediately
                background_tasks.add_task(run_pipeline_task, pipeline_service)
            elif action == "reject":
                logger.info(f"Article {article_id} rejected via LINE.")
                await repo.update_status(
                    article_id, ArticleUpdate(status=ArticleStatus.IGNORED)
                )

    return {"status": "ok"}


@app.get("/health")
async def health_check():
    """Simple health check endpoint."""
    return {"status": "ok"}
