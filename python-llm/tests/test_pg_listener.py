"""Tests for PostgreSQL LISTEN/NOTIFY listener service."""

import asyncio
import json
import pytest
from unittest.mock import AsyncMock, patch

from app.services.pg_listener import PgListenerService
from app.config import Config


@pytest.fixture
def config():
    cfg = Config()
    cfg.db.url = "postgresql://localhost/autotech"
    return cfg


@pytest.mark.asyncio
async def test_pg_listener_registers_and_triggers(config):
    """PgListenerService が通知受信時にコールバックを非同期で実行することを確認"""
    callback_data = []
    callback_event = asyncio.Event()

    async def mock_callback(data):
        callback_data.append(data)
        callback_event.set()

    service = PgListenerService(config, mock_callback)

    # asyncpg.connect のモック
    mock_conn = AsyncMock()
    with patch("asyncpg.connect", return_value=mock_conn):
        service.start()

        # リスナーの登録待機
        for _ in range(10):
            if mock_conn.add_listener.called:
                break
            await asyncio.sleep(0.01)

        mock_conn.add_listener.assert_called_once()
        args = mock_conn.add_listener.call_args
        assert args[0][0] == "new_article"
        handler = args[0][1]

        # 通知のシミュレーション
        payload = {"id": 123, "title": "Test Paper"}
        await handler(mock_conn, 1234, "new_article", json.dumps(payload))

        # コールバックが呼ばれたか確認
        await asyncio.wait_for(callback_event.wait(), timeout=1.0)
        assert len(callback_data) == 1
        assert callback_data[0]["id"] == 123

        await service.stop()


@pytest.mark.asyncio
async def test_pg_listener_reconnects_on_failure(config):
    """接続に失敗しても再試行することを確認"""
    service = PgListenerService(config, AsyncMock())

    # 1回目は失敗、2回目は成功するモック
    mock_conn = AsyncMock()
    with patch("asyncpg.connect", side_effect=[Exception("Connection failed"), mock_conn]):
        # リトライ間隔を短縮
        with patch("asyncio.sleep", side_effect=lambda d: asyncio.sleep(0) if d == 5 else asyncio.sleep(d)):
            service.start()

            # 接続成功まで待機
            for _ in range(50):
                if mock_conn.add_listener.called:
                    break
                await asyncio.sleep(0.05)

            assert mock_conn.add_listener.called
            await service.stop()
