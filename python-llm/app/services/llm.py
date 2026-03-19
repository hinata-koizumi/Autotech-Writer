"""LLM service for article triage and generation."""

import json
import logging
from typing import Optional

from openai import AsyncOpenAI
from anthropic import AsyncAnthropic

from app.config import Config

logger = logging.getLogger(__name__)


class LLMService:
    """Service to interact with LLMs for triage and content generation."""

    def __init__(self, config: Config):
        self.config = config
        
        # Initialize clients based on availability of keys
        self.openai_client = AsyncOpenAI(api_key=config.openai_api_key) if config.openai_api_key else None
        self.anthropic_client = AsyncAnthropic(api_key=config.anthropic_api_key) if config.anthropic_api_key else None

        # Determine which clients to use for which task
        # Preference: Anthropic for generation (Claude 3.5 Sonnet), OpenAI for fast triage (GPT-4o-mini)
        self.triage_provider = "openai" if self.openai_client else ("anthropic" if self.anthropic_client else None)
        self.gen_provider = "anthropic" if self.anthropic_client else ("openai" if self.openai_client else None)

    async def triage_article(self, title: str, summary: str, source_type: str) -> bool:
        """
        Determine if learning about this article is valuable for Japanese SWE/AIE.
        Returns True if valuable, False otherwise.
        """
        if self.config.dry_run and not self.triage_provider:
            # Fake successful triage in dry-run with no keys
            return True
            
        if not self.triage_provider:
            raise ValueError("No LLM clients configured for triage.")

        prompt = f"""
        あなたは日本のシニアソフトウェアエンジニア・AIエンジニアです。
        以下の技術記事やリリースノートが、日本の技術者にとって「詳細を解説する価値がある重要な情報か」を判定してください。
        価値がある場合は "true"、そうでない場合（些細なバグ修正、マイナーすぎるツール、非技術的な内容など）は "false" とだけ小文字で返答してください。

        Source: {source_type}
        Title: {title}
        Summary: {summary}
        """

        try:
            if self.triage_provider == "openai":
                response = await self.openai_client.chat.completions.create(
                    model="gpt-4o-mini",
                    messages=[{"role": "user", "content": prompt}],
                    temperature=0.0,
                    max_tokens=10,
                )
                result_text = response.choices[0].message.content.strip().lower()
            else:
                response = await self.anthropic_client.messages.create(
                    model="claude-3-haiku-20240307",
                    max_tokens=10,
                    temperature=0.0,
                    messages=[{"role": "user", "content": prompt}]
                )
                result_text = response.content[0].text.strip().lower()
            
            return "true" in result_text
        except Exception as e:
            logger.error(f"Error during triage: {e}")
            # Err on the side of caution (triage true) if request fails
            return True

    async def generate_article(self, title: str, summary: str, source_type: str) -> str:
        """
        Generate a long-form Japanese markdown article explaining the tech news.
        """
        if self.config.dry_run and not self.gen_provider:
            return f"【TL;DR】\nDry run for {title}\n【詳細】\n詳細な説明..."
            
        if not self.gen_provider:
            raise ValueError("No LLM clients configured for generation.")

        system_prompt = (
            "あなたは最新鋭のAI・ソフトウェア技術を分かりやすく解説する、経験豊富な日本のシニアエンジニアです。\n"
            "以下の技術論文またはリリースノートの情報を基に、X(旧Twitter)の長文ポストに最適な、"
            "1,500〜2,500文字程度の専門的なマークダウン記事を日本語で生成してください。\n"
            "**必ず以下のセクションを含めてください:**\n"
            "1. 【TL;DR】(3-4行で要点をまとめる)\n"
            "2. 【背景・課題】(なぜこの技術・発表が必要だったのか)\n"
            "3. 【提案手法・主な変更点】(アーキテクチャや技術的な工夫点)\n"
            "4. 【定量評価・パフォーマンス】(ベンチマーク数値や改善指標などの客観的な事実)\n\n"
            "**制約事項:**\n"
            "- ハルシネーションを完全に排除し、与えられた情報（客観的な事実、数値等）のみを記述すること。\n"
            "- アルゴリズムによるリーチ制限を回避するため、外部リンク（URL）は一切記述しないこと。\n"
            "- 技術用語は適宜英語のままか、一般的なカタカナ表記を使用すること。"
        )

        user_prompt = f"Source: {source_type}\nTitle: {title}\nSummary:\n{summary}"

        try:
            if self.gen_provider == "anthropic":
                response = await self.anthropic_client.messages.create(
                    model="claude-3-5-sonnet-20241022",
                    max_tokens=4000,
                    temperature=0.3, # Low temperature for factuality
                    system=system_prompt,
                    messages=[{"role": "user", "content": user_prompt}]
                )
                return response.content[0].text.strip()
            else:
                response = await self.openai_client.chat.completions.create(
                    model="gpt-4o",
                    messages=[
                        {"role": "system", "content": system_prompt},
                        {"role": "user", "content": user_prompt}
                    ],
                    temperature=0.3,
                    max_tokens=3000,
                )
                return response.choices[0].message.content.strip()
        except Exception as e:
            logger.error(f"Error generating article: {e}")
            raise
