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
from app.models import TriggerResponse

logging.basicConfig(
    level=logging.DEBUG,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# Global configuration
config = Config()

# Persistent resources
_db_pool: Optional[asyncpg.Pool] = None


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
            max_size=config.db.pool_max_size
        )
    except Exception as e:
        logger.error(f"Failed to connect to database: {e}")
        raise RuntimeError(f"Database connection failed: {e}") from e

    # 3. Initialize Services (Singletons)
    app.state.llm_service = LLMService(config)
    app.state.x_api = XApiService(config)

    yield

    # Shutdown
    logger.info("Shutting down Autotech Writer LLM API...")
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


def get_pipeline_service(
    repo: Annotated[ArticleRepository, Depends(get_repo)],
    llm: Annotated[LLMService, Depends(get_llm_service)],
    x_api: Annotated[XApiService, Depends(get_x_api)],
    cfg: Annotated[Config, Depends(get_config)],
) -> PipelineService:
    return PipelineService(repo, llm, x_api, cfg)


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


@app.get("/health")
async def health_check():
    """Simple health check endpoint."""
    return {"status": "ok"}
