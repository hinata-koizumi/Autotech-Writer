"""Database repository for article state management."""

import json
import logging
from enum import Enum
from typing import Optional

logger = logging.getLogger(__name__)


from app.models import ArticleStatus, ArticleUpdate


class ArticleRepository:
    """Manages article state transitions in the database."""

    def __init__(self, db_pool):
        self.db_pool = db_pool

    async def get_pending_articles(self, limit: int = 10) -> list[dict]:
        """
        Fetch articles with status 'pending' and mark them as 'processing' atomically.
        Uses SELECT FOR UPDATE SKIP LOCKED to prevent race conditions.
        """
        async with self.db_pool.acquire() as conn:
            async with conn.transaction():
                # 1. Select pending articles and lock them, ignoring already locked rows
                rows = await conn.fetch(
                    """
                    SELECT id, source_type, source_id, title, summary, url, published_at, 
                           article_body, last_posted_index, x_thread_ids, status
                    FROM articles 
                    WHERE status IN ($1, $3) 
                    ORDER BY created_at 
                    LIMIT $2 
                    FOR UPDATE SKIP LOCKED
                    """,
                    ArticleStatus.PENDING.value,
                    limit,
                    ArticleStatus.PARTIAL_FAILED.value,
                )

                if not rows:
                    return []

                # 2. Mark these specific articles as processing immediately
                article_ids = [row["id"] for row in rows]
                await conn.execute(
                    "UPDATE articles SET status = $1, updated_at = NOW() WHERE id = ANY($2)",
                    ArticleStatus.PROCESSING.value,
                    article_ids,
                )

                return [dict(row) for row in rows]

    async def get_full_content(self, article_id: int) -> Optional[str]:
        """Fetch the full content (LaTeX/PR diffs) for a specific article."""
        async with self.db_pool.acquire() as conn:
            return await conn.fetchval(
                "SELECT full_content FROM articles WHERE id = $1", article_id
            )

    async def update_status(
        self,
        article_id: int,
        update: ArticleUpdate,
    ) -> None:
        """Update article status and optional fields using ArticleUpdate model."""
        updates = {"updated_at": "NOW()"}
        query_params = []

        # Map update fields to database columns
        data = update.model_dump(exclude_unset=True)

        for key, value in data.items():
            if key == "status" and value is not None:
                updates["status"] = value.value if hasattr(value, "value") else value
            elif key == "x_thread_ids" and value is not None:
                updates["x_thread_ids"] = json.dumps(value)
                if value:
                    updates["x_post_id"] = value[0]
            elif value is not None:
                updates[key] = value

        # Construct query
        set_clauses = []
        for key, value in updates.items():
            if value == "NOW()":
                set_clauses.append(f"{key} = NOW()")
            else:
                query_params.append(value)
                set_clauses.append(f"{key} = ${len(query_params)}")

        query_params.append(article_id)
        query = f"UPDATE articles SET {', '.join(set_clauses)} WHERE id = ${len(query_params)}"

        async with self.db_pool.acquire() as conn:
            await conn.execute(query, *query_params)

    async def increment_retry(
        self, article_id: int, max_retries: int = 5
    ) -> ArticleStatus:
        """Increment retry count atomically and set appropriate status."""
        async with self.db_pool.acquire() as conn:
            new_status_val = await conn.fetchval(
                """
                UPDATE articles 
                SET retry_count = retry_count + 1, 
                    status = CASE WHEN retry_count + 1 >= $1 THEN $2 ELSE $3 END, 
                    updated_at = NOW() 
                WHERE id = $4
                RETURNING status
                """,
                max_retries,
                ArticleStatus.FAILED.value,
                ArticleStatus.RETRY.value,
                article_id,
            )
            if new_status_val is None:
                raise ValueError(f"Article {article_id} not found")
            return ArticleStatus(new_status_val)
