"""Service for interacting with X (Twitter) API v2."""

import logging
from typing import Dict, Any, Optional

import httpx
from app.config import Config

logger = logging.getLogger(__name__)


class XApiService:
    """Service to post articles to X."""

    def __init__(self, config: Config):
        self.config = config
        self.base_url = "https://api.twitter.com/2"

        # Using OAuth 2.0 Bearer token if provided (e.g. App-only or User Context if configured correctly)
        # Note: In production, for POST /2/tweets User Context (OAuth 1.0a or OAuth 2.0 Auth Code) is required.
        # This implementation uses the simplest httpx Bearer token approach for demonstration/TDD.
        self.client = httpx.AsyncClient(
            base_url=self.base_url,
            timeout=30.0,
        )

    async def post_article(self, text: str) -> Optional[str]:
        """
        Posts a long-form article to X (requires X Premium for >280 chars).
        Returns the tweet ID if successful.
        """
        if self.config.dry_run:
            logger.info("DRY-RUN: Simulated X API post.")
            logger.debug(f"DRY-RUN content:\n{text[:100]}...")
            if self.config.dry_run_webhook_url:
                # Can send to a local webhook for debugging instead
                try:
                    await self.client.post(self.config.dry_run_webhook_url, json={"text": text})
                except Exception as e:
                    logger.warning(f"Dry-run webhook failed: {e}")
            return "dry_run_post_id"

        if not self.config.x_access_token:
            raise ValueError("X_ACCESS_TOKEN not configured.")

        # Minimal implementation for posting a tweet via API v2.
        # Using OAuth 2.0 bearer (requires proper setup from developer portal).
        headers = {
            "Authorization": f"Bearer {self.config.x_access_token}",
            "Content-Type": "application/json",
        }
        
        payload = {"text": text}

        try:
            response = await self.client.post("/tweets", json=payload, headers=headers)
            
            # Rate limit check can be added here
            if response.status_code == 429:
                logger.error("Rate limited by X API.")
                raise httpx.HTTPStatusError("Rate Limit", request=response.request, response=response)

            response.raise_for_status()
            
            data = response.json()
            return data.get("data", {}).get("id")
            
        except httpx.HTTPError as e:
            logger.error(f"HTTP error posting to X: {e}")
            raise
        except Exception as e:
            logger.error(f"Unexpected error posting to X: {e}")
            raise

    async def close(self):
        """Close the HTTP client session."""
        await self.client.aclose()
