from app.models import (
    ExtractionResult,
    ArticleResponse,
)


def mock_extraction_response() -> ExtractionResult:
    return ExtractionResult(
        arxiv_id="2407.02071",
        target_hardware="Hopper世代GPU (H100)",
        primary_result="最大2.0倍の高速化を達成",
        problem_statement="LLM推論コストの高さ",
        proposed_architecture="New Quantization",
        technical_keyword="Asynchronous execution",
        evaluation_metrics=[],
        limitations="N/A",
        current_status="Research Preview",
        extraction_confidence=1.0,
        reason_for_insufficient="",
        is_information_sufficient=True,
    )


def mock_article_response() -> ArticleResponse:
    return ArticleResponse(
        content="""最大2.0倍の高速化を達成 (H100)
arXiv:2407.02071
Hopper世代GPU利用者向け。
[IMAGE: mock_benchmark]
---
旧バージョンFA2との比較:...
[IMAGE: mock_compare]
---
演算とメモリアクセスを80%オーバーラップした結果、Asynchronous executionが実現。
[IMAGE: mock_tech]
---
詳細ベンチマーク: シーケンス長128kまで対応。
[IMAGE: mock_bench_detail]
---
実務影響: H100推奨。現在はResearch Preview段階。
[IMAGE: mock_impact]
---
参考リンク:
- https://arxiv.org/abs/2407.02071
- https://github.com/dao-ai/flash-attention
試した方の体感値を募集中です。
[IMAGE: mock_links]"""
    )
