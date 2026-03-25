import asyncio
import logging
from typing import Callable, Coroutine, Any, Optional

import asyncpg
from app.config import Config

logger = logging.getLogger(__name__)


class PgListenerService:
    """
    Service that listens for PostgreSQL notifications and triggers actions.
    Uses a dedicated connection to LISTEN on a specific channel.
    """

    def __init__(
        self, config: Config, trigger_callback: Callable[[], Coroutine[Any, Any, None]]
    ):
        self.config = config
        self.trigger_callback = trigger_callback
        self._conn: Optional[asyncpg.Connection] = None
        self._task: Optional[asyncio.Task] = None
        self._stop_event = asyncio.Event()

    async def _handle_notification(
        self, connection: asyncpg.Connection, pid: int, channel: str, payload: str
    ):
        """Callback for when a notification is received."""
        logger.info(f"Received PG notification on '{channel}': {payload}")
        try:
            # Parse the JSON payload from the trigger
            data = {}
            if payload:
                try:
                    import json

                    data = json.loads(payload)
                except Exception:
                    logger.warning(
                        f"Failed to parse PG notification payload: {payload}"
                    )

            # Trigger the pipeline callback with the data
            if asyncio.iscoroutinefunction(self.trigger_callback):
                asyncio.create_task(self.trigger_callback(data))
            else:
                # Handle non-async callback if any
                self.trigger_callback(data)
        except Exception as e:
            logger.error(f"Error during trigger callback execution: {e}")

    async def _listen_loop(self):
        """Continuous loop to handle reconnection and listening."""
        while not self._stop_event.is_set():
            try:
                # Dedicated connection for LISTEN/NOTIFY
                logger.debug(f"Connecting to {self.config.db.url} for LISTEN...")
                self._conn = await asyncpg.connect(self.config.db.url)

                await self._conn.add_listener("new_article", self._handle_notification)
                logger.info("PG Listener: Registered 'new_article' channel.")

                # Keep the connection alive until stopped or closed
                while not self._stop_event.is_set():
                    # Check if connection is still alive
                    try:
                        await self._conn.execute("SELECT 1")
                    except Exception:
                        logger.warning("PG Listener connection lost. Reconnecting...")
                        break
                    await asyncio.sleep(30)  # Heartbeat interval

            except asyncio.CancelledError:
                break
            except Exception as e:
                if not self._stop_event.is_set():
                    logger.error(f"PG Listener error: {e}. Retrying in 5s...")
                    await asyncio.sleep(5)
            finally:
                if self._conn and not self._conn.is_closed():
                    try:
                        await self._conn.remove_listener(
                            "new_article", self._handle_notification
                        )
                        await self._conn.close()
                    except Exception:
                        pass
                self._conn = None

    def start(self):
        """Starts the background listener task."""
        if self._task is None or self._task.done():
            self._stop_event.clear()
            self._task = asyncio.create_task(self._listen_loop())
            logger.info("PgListenerService started.")

    async def stop(self):
        """Stops the background listener task."""
        logger.info("Stopping PgListenerService...")
        self._stop_event.set()
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
            self._task = None
        logger.info("PgListenerService stopped.")
