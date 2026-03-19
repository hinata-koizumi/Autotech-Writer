"""Tests for X API payload sanitizer."""

import pytest

from app.services.sanitizer import sanitize_for_x_api


class TestSanitizer:
    """Tests for sanitize_for_x_api function."""

    def test_clean_text_unchanged(self):
        """[正常系] クリーンなテキストはそのまま返されること"""
        text = "これは正常なテキストです。\n改行も含まれています。"
        result = sanitize_for_x_api(text)
        assert result == text

    def test_excessive_newlines_normalized(self):
        """[異常系] 過剰な改行が正規化されること（3つ以上→2つ）"""
        text = "段落1\n\n\n\n\n段落2"
        result = sanitize_for_x_api(text)
        assert result == "段落1\n\n段落2"

    def test_double_newline_preserved(self):
        """[正常系] 2つの改行は保持されること"""
        text = "段落1\n\n段落2"
        result = sanitize_for_x_api(text)
        assert result == "段落1\n\n段落2"

    def test_consecutive_special_symbols_reduced(self):
        """[異常系] 特殊記号の連続が適切に削減されること"""
        text = "注目!!!!!これは重要です"
        result = sanitize_for_x_api(text)
        assert "!!!!!" not in result
        assert "!!" in result  # 2つまでに削減

    def test_null_bytes_removed(self):
        """[異常系] ヌルバイトが除去されること"""
        text = "テスト\x00テキスト\x00です"
        result = sanitize_for_x_api(text)
        assert "\x00" not in result
        assert "テストテキストです" == result

    def test_control_chars_removed(self):
        """[異常系] 制御文字が除去されること"""
        text = "テスト\x01\x02\x03テキスト\x7f"
        result = sanitize_for_x_api(text)
        assert "\x01" not in result
        assert "\x02" not in result
        assert "\x7f" not in result

    def test_whitespace_trimmed(self):
        """[異常系] 前後の空白が除去されること"""
        text = "   テスト   "
        result = sanitize_for_x_api(text)
        assert result == "テスト"

    def test_multiple_hash_symbols(self):
        """[異常系] ハッシュ記号の連続が削減されること"""
        text = "トピック #### 重要"
        result = sanitize_for_x_api(text)
        assert "####" not in result

    def test_mixed_issues(self):
        """[異常系] 複数の問題が同時に修正されること"""
        text = "\x00  テスト!!!!!!\n\n\n\n本文\x01  "
        result = sanitize_for_x_api(text)
        assert "\x00" not in result
        assert "\x01" not in result
        assert "!!!!!!" not in result
        assert "\n\n\n\n" not in result

    def test_japanese_text_preserved(self):
        """[正常系] 日本語テキストが正しく保持されること"""
        text = "🔥 新しいLLMアーキテクチャ「Mamba-2」が発表されました！\n性能はTransformerと同等。"
        result = sanitize_for_x_api(text)
        assert "Mamba-2" in result
        assert "🔥" in result

    def test_empty_string(self):
        """[正常系] 空文字列が安全に処理されること"""
        result = sanitize_for_x_api("")
        assert result == ""
