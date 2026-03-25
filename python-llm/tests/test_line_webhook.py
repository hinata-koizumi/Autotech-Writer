import pytest
from unittest.mock import AsyncMock, patch, MagicMock
from fastapi.testclient import TestClient
from app.main import app
from app.models import ArticleStatus

# Use TestClient for synchronous-style testing of FastAPI endpoints
client = TestClient(app)


@pytest.mark.asyncio
async def test_line_webhook_approve():
    """
    Test that the LINE webhook correctly handles an 'approve' postback.
    It should update the article status to APPROVED and trigger the pipeline.
    """
    mock_repo = MagicMock()
    mock_repo.update_status = AsyncMock()

    # Mock a LINE PostbackEvent
    # Note: We need to mock the event object structure expected by app/main.py
    mock_event = MagicMock()
    # Mocking the attribute access event.postback.data
    mock_event.postback.data = "action=approve&article_id=123"

    # We need to ensure isinstance(event, PostbackEvent) returns True in the test
    from linebot.v3.webhooks import PostbackEvent

    with patch("app.main.WebhookParser.parse", return_value=[mock_event]), patch(
        "app.main.get_repo", return_value=mock_repo
    ), patch("app.main.PostbackEvent", PostbackEvent), patch(
        "app.main.isinstance",
        side_effect=lambda obj, cls: (
            True if cls == PostbackEvent else isinstance(obj, cls)
        ),
    ):

        response = client.post(
            "/line/webhook",
            headers={"X-Line-Signature": "dummy_signature"},
            content="dummy_body",
        )

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}

    # Verify status update
    mock_repo.update_status.assert_called_once()
    args, kwargs = mock_repo.update_status.call_args
    assert args[0] == 123
    assert args[1].status == ArticleStatus.APPROVED


@pytest.mark.asyncio
async def test_line_webhook_reject():
    """
    Test that the LINE webhook correctly handles a 'reject' postback.
    It should update the article status to IGNORED.
    """
    mock_repo = MagicMock()
    mock_repo.update_status = AsyncMock()

    mock_event = MagicMock()
    mock_event.postback.data = "action=reject&article_id=456"

    from linebot.v3.webhooks import PostbackEvent

    with patch("app.main.WebhookParser.parse", return_value=[mock_event]), patch(
        "app.main.get_repo", return_value=mock_repo
    ), patch("app.main.PostbackEvent", PostbackEvent), patch(
        "app.main.isinstance",
        side_effect=lambda obj, cls: (
            True if cls == PostbackEvent else isinstance(obj, cls)
        ),
    ):

        response = client.post(
            "/line/webhook",
            headers={"X-Line-Signature": "dummy_signature"},
            content="dummy_body",
        )

    assert response.status_code == 200

    # Verify status update
    mock_repo.update_status.assert_called_once()
    args, kwargs = mock_repo.update_status.call_args
    assert args[0] == 456
    assert args[1].status == ArticleStatus.IGNORED


def test_line_webhook_invalid_signature():
    """
    Test that the LINE webhook returns 400 on an invalid signature.
    """
    from linebot.v3.exceptions import InvalidSignatureError

    with patch("app.main.WebhookParser.parse", side_effect=InvalidSignatureError):
        response = client.post(
            "/line/webhook", headers={"X-Line-Signature": "wrong"}, content="any"
        )

    assert response.status_code == 400
