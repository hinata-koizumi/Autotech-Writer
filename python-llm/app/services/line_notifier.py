import logging
import asyncio
from linebot.v3.messaging import (
    Configuration,
    ApiClient,
    MessagingApi,
    PushMessageRequest,
    TemplateMessage,
    ConfirmTemplate,
    PostbackAction,
)
from app.config import Config

logger = logging.getLogger(__name__)


class LineNotifierService:
    """
    Service for sending notifications and approval requests via LINE Messaging API.
    Uses ConfirmTemplate with PostbackActions for interactive Approve/Reject buttons.
    """

    def __init__(self, config: Config):
        self.config = config
        self.line_config = Configuration(access_token=config.line.channel_access_token)

    async def send_approval_request(self, article_id: int, title: str):
        """
        Sends an interactive approval request to the configured LINE User ID.
        """
        if not self.config.line.channel_access_token or not self.config.line.user_id:
            logger.warning(
                "LINE credentials or User ID not configured. Skipping notification."
            )
            return

        # Truncate title for LINE display limits
        display_title = (title[:57] + "...") if len(title) > 60 else title

        # Create ConfirmTemplate with PostbackActions
        confirm_template = ConfirmTemplate(
            text=f"新着記事の承認依頼:\n{display_title}",
            actions=[
                PostbackAction(
                    label="承認 (Post)",
                    data=f"action=approve&article_id={article_id}",
                    display_text="承認されました。投稿準備に入ります。",
                ),
                PostbackAction(
                    label="却下 (Reject)",
                    data=f"action=reject&article_id={article_id}",
                    display_text="記事を却下しました。",
                ),
            ],
        )

        template_message = TemplateMessage(
            alt_text="記事承認の依頼", template=confirm_template
        )

        push_message_request = PushMessageRequest(
            to=self.config.line.user_id, messages=[template_message]
        )

        try:
            # The SDK calls are blocking (requests-based), so we run in a thread
            # to avoid blocking the async event loop if needed, but for a single
            # push, a direct call is usually fine in this pipeline context.
            def _push():
                with ApiClient(self.line_config) as api_client:
                    line_bot_api = MessagingApi(api_client)
                    line_bot_api.push_message(push_message_request)

            await asyncio.to_thread(_push)
            logger.info(
                f"Successfully sent LINE approval request for article {article_id}"
            )

        except Exception as e:
            logger.error(f"Failed to send LINE message for article {article_id}: {e}")
            # We don't raise here to avoid crashing the pipeline;
            # logging the failure is sufficient as the article remains in WAITING_APPROVAL.
