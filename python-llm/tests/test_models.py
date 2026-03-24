"""Tests for Pydantic model validation of current models."""

import pytest
from pydantic import ValidationError
from app.models import ExtractionResult, ArticleResponse, EvaluationMetric


class TestExtractionResult:
    """Tests for ExtractionResult model."""

    def test_valid_extraction(self):
        """[正常系] 全フィールドが正しくパースされること"""
        data = {
            "arxiv_id": "2407.02071",
            "model_size": "7B",
            "target_hardware": "H100",
            "primary_result": "2.0x faster",
            "problem_statement": "High cost",
            "proposed_architecture": "New Quant",
            "technical_keyword": "Asynchronous",
            "evaluation_metrics": [
                {"metric_name": "Latency", "score": "0.5ms", "source_quote": "result is 0.5ms"}
            ],
            "limitations": "N/A",
            "current_status": "Preview",
            "is_information_sufficient": True
        }
        result = ExtractionResult(**data)
        assert result.arxiv_id == "2407.02071"
        assert len(result.evaluation_metrics) == 1
        assert result.evaluation_metrics[0].metric_name == "Latency"

    def test_optional_fields(self):
        """[正常系] オプショナルなフィールドが欠けていてもデフォルト値が入ること"""
        result = ExtractionResult(primary_result="Success")
        assert result.arxiv_id is None
        assert result.model_size == ""
        assert result.is_information_sufficient is False


class TestArticleResponse:
    """Tests for ArticleResponse model."""

    def test_valid_article(self):
        """[正常系] contentが正しく保持されること"""
        result = ArticleResponse(content="This is a test article.")
        assert result.content == "This is a test article."

    def test_missing_content_raises(self):
        """[異常系] contentが欠けている場合にValidationErrorが発生すること"""
        with pytest.raises(ValidationError):
            ArticleResponse()


class TestEvaluationMetric:
    """Tests for EvaluationMetric model."""

    def test_valid_metric(self):
        """[正常系] 各型が正しく扱われること"""
        metric = EvaluationMetric(
            metric_name="Throughput",
            score=100.5,
            source_quote="100.5 tokens/s"
        )
        assert metric.score == 100.5

    def test_string_score(self):
        """[正常系] scoreに文字列が渡されても許容されること"""
        metric = EvaluationMetric(
            metric_name="Accuracy",
            score="95%",
            source_quote="95% accuracy achieved"
        )
class TestArticleUpdate:
    """Tests for ArticleUpdate model."""

    def test_valid_update(self):
        """[正常系] フィールドが正しく設定されること"""
        from app.models import ArticleStatus, ArticleUpdate
        update = ArticleUpdate(
            status=ArticleStatus.COMPLETED,
            hook_text="Test hook",
            x_thread_ids=["123", "456"]
        )
        assert update.status == ArticleStatus.COMPLETED
        assert update.hook_text == "Test hook"
        assert update.x_thread_ids == ["123", "456"]

    def test_empty_update(self):
        """[正常系] 全フィールドがNoneでもバリデーションに通ること"""
        from app.models import ArticleUpdate
        update = ArticleUpdate()
        assert update.status is None
        assert update.hook_text is None
