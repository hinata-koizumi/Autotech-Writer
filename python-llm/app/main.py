"""FastAPI application for Autotech Writer LLM Module."""

import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, BackgroundTasks, HTTPException
import asyncpg

from app.config import Config
from app.repository import ArticleRepository
from app.services.llm import LLMService
from app.services.x_api import XApiService
from app.pipeline.pipeline import process_article

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

config = Config()

# Global state
app_state = {}


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifecycle events for FastAPI app."""
    logger.info("Starting Autotech Writer LLM API...")
    
    # Initialize DB pool
    try:
        pool = await asyncpg.create_pool(dsn=config.database_url, min_size=1, max_size=5)
        app_state["db_pool"] = pool
    except Exception as e:
        logger.error(f"Failed to connect to database: {e}")
        raise RuntimeError(f"Database connection failed: {e}")

    # Initialize Services
    app_state["repo"] = ArticleRepository(pool)
    app_state["llm_service"] = LLMService(config)
    app_state["x_api"] = XApiService(config)

    yield

    # Shutdown
    logger.info("Shutting down Autotech Writer LLM API...")
    await app_state["x_api"].close()
    await app_state["db_pool"].close()


app = FastAPI(title="Autotech Writer LLM API", lifespan=lifespan)


async def process_pending_articles():
    """Background task to fetch and process all pending articles."""
    repo: ArticleRepository = app_state["repo"]
    llm_service: LLMService = app_state["llm_service"]
    x_api: XApiService = app_state["x_api"]

    try:
        articles = await repo.get_pending_articles(limit=10)
        logger.info(f"Found {len(articles)} pending articles.")

        for article in articles:
            await process_article(article, repo, llm_service, x_api, config)
            # Add small delay between articles to avoid rate limits
            await asyncio.sleep(2)
            
    except Exception as e:
        logger.error(f"Error in process_pending_articles background task: {e}")


@app.post("/trigger")
async def trigger_pipeline(background_tasks: BackgroundTasks):
    """
    Endpoint triggered by the Go collector module or chron.
    Starts processing of pending articles in the background.
    """
    logger.info("Received request to trigger pipeline.")
    background_tasks.add_task(process_pending_articles)
    return {"status": "Processing started."}


@app.get("/health")
async def health_check():
    """Simple health check endpoint."""
    return {"status": "ok"}
