"""Tests for Pydantic model validation."""

import pytest
from pydantic import ValidationError

from app.models import GeneratedArticle, TriageResult


class TestTriageResult:
    """Tests for TriageResult model."""

    def test_valid_true(self):
        """[正常系] is_valuable=True が正しくパースされること"""
        result = TriageResult(is_valuable=True)
        assert result.is_valuable is True

    def test_valid_false(self):
        """[正常系] is_valuable=False が正しくパースされること"""
        result = TriageResult(is_valuable=False)
        assert result.is_valuable is False

    def test_from_json(self):
        """[正常系] JSON辞書からパースできること"""
        data = {"is_valuable": True}
        result = TriageResult(**data)
        assert result.is_valuable is True


class TestGeneratedArticle:
    """Tests for GeneratedArticle model with validation rules."""

    @staticmethod
    def _make_body(length: int = 1500) -> str:
        """Helper to create article body of specified length."""
        return "あ" * length

    def test_valid_article(self):
        """[正常系] hook_text と article_body を含むLLMレスポンスが正しくパースされること"""
        article = GeneratedArticle(
            hook_text="🔥 新しいTransformerアーキテクチャが登場",
            article_body=self._make_body(1500),
        )
        assert article.hook_text == "🔥 新しいTransformerアーキテクチャが登場"
        assert len(article.article_body) == 1500

    def test_hook_text_with_http_url_raises(self):
        """[異常系] hook_textにhttp://を含むURLがある場合ValidationErrorが発生すること"""
        with pytest.raises(ValidationError) as exc_info:
            GeneratedArticle(
                hook_text="詳しくは http://example.com を参照",
                article_body=self._make_body(),
            )
        assert "url" in str(exc_info.value).lower() or "URL" in str(exc_info.value)

    def test_hook_text_with_https_url_raises(self):
        """[異常系] hook_textにhttps://を含むURLがある場合ValidationErrorが発生すること"""
        with pytest.raises(ValidationError) as exc_info:
            GeneratedArticle(
                hook_text="参考: https://arxiv.org/abs/2401.00001",
                article_body=self._make_body(),
            )
        assert "url" in str(exc_info.value).lower() or "URL" in str(exc_info.value)

    def test_article_body_with_url_raises(self):
        """[異常系] article_bodyにURLが含まれる場合ValidationErrorが発生すること"""
        body_with_url = self._make_body(500) + " https://github.com/test " + self._make_body(500)
        with pytest.raises(ValidationError):
            GeneratedArticle(
                hook_text="テスト",
                article_body=body_with_url,
            )

    def test_article_body_too_short_raises(self):
        """[異常系] article_bodyが1000文字未満の場合エラーとして弾くこと"""
        with pytest.raises(ValidationError) as exc_info:
            GeneratedArticle(
                hook_text="テスト",
                article_body="短すぎる本文",  # 6 chars
            )
        assert "1000" in str(exc_info.value)

    def test_article_body_exactly_1000_chars(self):
        """[正常系] article_bodyがちょうど1000文字の場合はパスすること"""
        article = GeneratedArticle(
            hook_text="テスト",
            article_body=self._make_body(1000),
        )
        assert len(article.article_body) == 1000

    def test_article_body_999_chars_raises(self):
        """[異常系] article_bodyが999文字の場合エラーになること"""
        with pytest.raises(ValidationError):
            GeneratedArticle(
                hook_text="テスト",
                article_body=self._make_body(999),
            )

    def test_url_in_middle_of_text_detected(self):
        """[異常系] テキスト中間に埋め込まれたURLも検出されること"""
        with pytest.raises(ValidationError):
            GeneratedArticle(
                hook_text="前文 http://evil.com 後文",
                article_body=self._make_body(),
            )

    def test_text_without_url_passes(self):
        """[正常系] URLを含まないテキストはバリデーションを通過すること"""
        article = GeneratedArticle(
            hook_text="これはURLを含まないテキストです。httpという単語だけなら大丈夫",
            article_body=self._make_body(),
        )
        assert "http" in article.hook_text  # "http" alone without :// is OK
