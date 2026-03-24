"""LLM-as-Judge tests for evaluating generated article factuality."""

import json
import logging
import os
import pytest
from pathlib import Path

# Skip test if API keys mapped for actual generation/evaluation are missing
# In CI, we can use VCR.py or similar to mock these, or just skip if no keys
pytestmark = pytest.mark.skipif(
    not os.getenv("OPENAI_API_KEY") and not os.getenv("ANTHROPIC_API_KEY"),
    reason="LLM API keys not provided for LLM-as-Judge evaluation",
)


def load_testdata():
    """Load fact evaluation dataset."""
    dataset_path = (
        Path(__file__).parent.parent.parent / "testdata" / "fact_eval_dataset.json"
    )
    with open(dataset_path, "r", encoding="utf-8") as f:
        return json.load(f)


# Placeholder for actual LLM generation function we will implement later
async def mock_generate_article(title: str, summary: str, source_type: str) -> str:
    """Mock generator mapping predefined outputs for testing the judge."""
    if "Scaling Laws" in title:
        return (
            "【TL;DR】\n"
            "自己回帰生成モデルのスケーリング則が経験的に調査されました。\n"
            "性能はパラメータ数(N)、データセットサイズ(D)、計算量(C)のスケールに強く依存し、"
            "モデルの形状には弱く依存することが判明しました。\n"
            "【詳細】..."
        )
    elif "v2.0.0" in title:
        return (
            "【TL;DR】\n"
            "v2.0.0では非同期I/Oエンジンが完全に書き直され、パフォーマンスが最大3倍向上しました。\n"
            "Python 3.12サポート追加の一方で、3.8はサポート終了。CVE-2024-1234（RSA脆弱性）のパッチも含まれます。\n"
        )
    return "Dummy response"


async def judge_factuality(
    article_text: str, expected_facts: list[str]
) -> tuple[bool, str]:
    """
    LLM-as-Judge: Evaluates if the generated article covers the expected facts.
    In a real implementation, this would call GPT-4o-mini or Claude-3-Haiku.
    """
    # For TDD purposes, we implement a simple keyword check to mock the LLM judge
    # which we can replace with a real generic Langchain/OpenAI call later.
    missing_facts = []

    # Very naive fact checking for test setup
    for fact in expected_facts:
        keywords = {
            "自己回帰生成モデルの自己回帰則（スケーリング則）を経験的に調査した": [
                "自己回帰",
                "経験的",
                "調査",
            ],
            "性能はモデルのスケール（パラメータ数N、データセットサイズD、計算量C）に強く依存する": [
                "パラメータ",
                "計算量",
                "スケール",
            ],
            "モデルの形状には弱く依存する": ["形状", "弱"],
            "メジャーリリースv2.0.0で非同期I/Oエンジンが完全に書き直された": [
                "非同期",
                "書き直され",
            ],
            "高並行処理のシナリオにおいて、パフォーマンスが最大3倍向上した": [
                "3倍",
                "パフォーマンス",
            ],
            "Python 3.8のサポートを終了し、Python 3.12のサポートを追加した": [
                "3.8",
                "終了",
                "3.12",
                "追加",
            ],
            "CVE-2024-1234（RSA暗号化の脆弱性）に対するセキュリティパッチが含まれている": [
                "CVE-2024-1234",
                "RSA",
                "脆弱性",
                "パッチ",
            ],
        }.get(fact, [])

        matches = sum(1 for kw in keywords if kw in article_text)
        if len(keywords) > 0 and matches < len(keywords) * 0.5:
            missing_facts.append(fact)

    score = 1.0 - (len(missing_facts) / len(expected_facts))

    if score >= 0.9:
        return True, "Passed: Substantially all facts covered"
    else:
        return False, f"Failed: Missing facts - {missing_facts}"


@pytest.mark.asyncio
async def test_article_factuality():
    """Evaluate article generation against ground truth facts."""
    dataset = load_testdata()

    for item in dataset:
        # 1. Generate article using our service logic (mocked here until implemented)
        generated_text = await mock_generate_article(
            title=item["title"], summary=item["summary"], source_type=item["source"]
        )

        # 2. Evaluate using LLM-as-Judge logic
        passed, reason = await judge_factuality(generated_text, item["expected_facts"])

        # 3. Assert passing score
        assert passed is True, f"Factuality check failed for {item['id']}: {reason}"
