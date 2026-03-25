import os
import pytest
import pytest_asyncio
import asyncpg
import asyncio
import json
from app.repository import ArticleRepository
from app.models import (
    ArticleStatus,
    ArticleUpdate,
)


@pytest_asyncio.fixture
async def db_pool():
    """Fixture to provide a database pool connected to the test database."""
    dsn = os.getenv("TEST_DB_DSN")
    if not dsn:
        pytest.skip("TEST_DB_DSN not set, skipping DB integration tests")

    pool = await asyncpg.create_pool(dsn)

    # Setup - clean test data before tests
    async with pool.acquire() as conn:
        await conn.execute("DELETE FROM articles WHERE source_type = 'test'")

    yield pool

    # Teardown - clean up test data after tests
    async with pool.acquire() as conn:
        await conn.execute("DELETE FROM articles WHERE source_type = 'test'")
    await pool.close()


@pytest.fixture
def repo(db_pool):
    """Fixture to provide the ArticleRepository instance."""
    return ArticleRepository(db_pool)


@pytest_asyncio.fixture
async def sample_article_id(db_pool):
    """Insert a sample pending article and return its ID."""
    async with db_pool.acquire() as conn:
        row = await conn.fetchrow("""
            INSERT INTO articles (source_type, source_id, title, summary, url, status, created_at)
            VALUES ('test', 'item-1', 'Test Title', 'Test Summary', 'http://test.com', 'pending', '2000-01-01')
            RETURNING id
            """)
        return row["id"]


@pytest.mark.asyncio
class TestArticleRepositoryIntegration:
    """Integration tests covering real database interactions for ArticleRepository."""

    async def test_get_pending_articles(self, repo, sample_article_id):
        """Test that get_pending_articles successfully retrieves pending rows."""
        pending = await repo.get_pending_articles(limit=10)

        # Verify the inserted row is returned
        test_articles = [p for p in pending if p["source_type"] == "test"]
        assert len(test_articles) == 1
        assert test_articles[0]["id"] == sample_article_id
        assert test_articles[0]["source_id"] == "item-1"

    async def test_update_status(self, db_pool, repo, sample_article_id):
        """Test status transitions to processing."""
        await repo.update_status(
            sample_article_id,
            ArticleUpdate(status=ArticleStatus.PROCESSING),
        )

        async with db_pool.acquire() as conn:
            updated = await conn.fetchrow(
                "SELECT status FROM articles WHERE id = $1",
                sample_article_id,
            )
            assert updated["status"] == ArticleStatus.PROCESSING.value

    async def test_increment_retry(self, db_pool, repo, sample_article_id):
        """Test the retry incrementer correctly updates status to RETRY or FAILED."""
        # 1st retry -> 'retry'
        status = await repo.increment_retry(sample_article_id, max_retries=3)
        assert status == ArticleStatus.RETRY

        async with db_pool.acquire() as conn:
            updated = await conn.fetchrow(
                "SELECT status, retry_count FROM articles WHERE id = $1",
                sample_article_id,
            )
            assert updated["status"] == ArticleStatus.RETRY.value
            assert updated["retry_count"] == 1

        # 2nd retry -> 'retry'
        await repo.increment_retry(sample_article_id, max_retries=3)

        # 3rd retry (reaches max) -> 'failed'
        status_failed = await repo.increment_retry(sample_article_id, max_retries=3)
        assert status_failed == ArticleStatus.FAILED

        async with db_pool.acquire() as conn:
            updated_failed = await conn.fetchrow(
                "SELECT status FROM articles WHERE id = $1",
                sample_article_id,
            )
            assert updated_failed["status"] == ArticleStatus.FAILED.value

    async def test_notify_trigger(self, db_pool):
        """Test that inserting an article triggers a NOTIFY event on 'new_article' channel."""
        notifications = []

        async def callback(connection, pid, channel, payload):
            notifications.append(payload)

        async with db_pool.acquire() as conn:
            # Start listening
            await conn.add_listener("new_article", callback)

            # Insert a row
            await conn.execute("""
                INSERT INTO articles (source_type, source_id, title, summary, url, status)
                VALUES ('test', 'notify-1', 'Notify Test', 'Summary', 'http://test.com', 'pending')
            """)

            # Wait a bit for notification (max 1 second)
            for _ in range(10):
                if notifications:
                    break
                await asyncio.sleep(0.1)

            assert len(notifications) > 0
            payload = json.loads(notifications[0])
            assert payload["source_type"] == "test"
            assert payload["title"] == "Notify Test"
            assert "id" in payload
