"""Database repository for article state management."""

from enum import Enum
from typing import Optional


class ArticleStatus(str, Enum):
    """Valid status values for articles."""

    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    RETRY = "retry"
    IGNORED = "ignored"
    REJECTED_COMPLIANCE = "rejected_compliance"


class ArticleRepository:
    """Manages article state transitions in the database."""

    def __init__(self, db_pool):
        self.db_pool = db_pool

    async def get_pending_articles(self, limit: int = 10) -> list[dict]:
        """Fetch articles with status 'pending'."""
        async with self.db_pool.acquire() as conn:
            rows = await conn.fetch(
                "SELECT * FROM articles WHERE status = $1 ORDER BY created_at LIMIT $2",
                ArticleStatus.PENDING.value,
                limit,
            )
            return [dict(row) for row in rows]

    async def update_status(
        self,
        article_id: int,
        new_status: ArticleStatus,
        hook_text: Optional[str] = None,
        article_body: Optional[str] = None,
        x_post_id: Optional[str] = None,
    ) -> None:
        """Update article status and optional fields."""
        async with self.db_pool.acquire() as conn:
            sets = ["status = $1", "updated_at = NOW()"]
            params = [new_status.value]
            idx = 2

            if hook_text is not None:
                sets.append(f"hook_text = ${idx}")
                params.append(hook_text)
                idx += 1

            if article_body is not None:
                sets.append(f"article_body = ${idx}")
                params.append(article_body)
                idx += 1

            if x_post_id is not None:
                sets.append(f"x_post_id = ${idx}")
                params.append(x_post_id)
                idx += 1

            sets_str = ", ".join(sets)
            params.append(article_id)
            await conn.execute(
                f"UPDATE articles SET {sets_str} WHERE id = ${idx}",
                *params,
            )

    async def increment_retry(self, article_id: int, max_retries: int = 5) -> ArticleStatus:
        """Increment retry count and set appropriate status."""
        async with self.db_pool.acquire() as conn:
            row = await conn.fetchrow(
                "SELECT retry_count FROM articles WHERE id = $1", article_id
            )
            if row is None:
                raise ValueError(f"Article {article_id} not found")

            new_count = row["retry_count"] + 1
            if new_count >= max_retries:
                new_status = ArticleStatus.FAILED
            else:
                new_status = ArticleStatus.RETRY

            await conn.execute(
                "UPDATE articles SET retry_count = $1, status = $2, updated_at = NOW() WHERE id = $3",
                new_count,
                new_status.value,
                article_id,
            )
            return new_status
