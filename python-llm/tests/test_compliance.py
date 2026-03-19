"""Tests for compliance filter (NG keyword detection)."""

import pytest

from app.services.compliance import check_compliance, NG_KEYWORDS


class TestComplianceFilter:
    """Tests for political/geopolitical NG word detection."""

    def test_clean_text_passes(self):
        """[正常系] 技術的なテキストはコンプライアンスチェックを通過すること"""
        text = (
            "新しいTransformerアーキテクチャが提案されました。"
            "ベンチマークではGPT-4oを上回る性能を示しています。"
            "学習率のスケジューリングにcosine annealing with warm restartsを採用。"
        )
        is_clean, detected = check_compliance(text)
        assert is_clean is True
        assert detected == []

    def test_japanese_ng_word_detected(self):
        """[異常系] 日本語のNGワード（戦争）が検出されること"""
        text = "この技術は戦争にも転用可能である。"
        is_clean, detected = check_compliance(text)
        assert is_clean is False
        assert "戦争" in detected

    def test_english_ng_word_detected(self):
        """[異常系] 英語のNGワード（terrorism）が検出されること"""
        text = "The model could be misused for terrorism activities."
        is_clean, detected = check_compliance(text)
        assert is_clean is False
        assert "terrorism" in detected

    def test_multiple_ng_words_detected(self):
        """[異常系] 複数のNGワードが同時に検出されること"""
        text = "軍事的な侵攻による紛争が発生。"
        is_clean, detected = check_compliance(text)
        assert is_clean is False
        assert len(detected) >= 2

    def test_case_insensitive_detection(self):
        """[異常系] 大文字小文字を問わずNGワードを検出すること"""
        text = "Discussion about SANCTIONS and INVASION policies."
        is_clean, detected = check_compliance(text)
        assert is_clean is False

    def test_partial_match_in_longer_word(self):
        """NGワードがより長い単語の一部として含まれる場合も検出される (仕様通り)"""
        text = "これはwar関連の記事です。"
        is_clean, detected = check_compliance(text)
        assert is_clean is False
        assert "war" in detected

    def test_empty_text_passes(self):
        """[正常系] 空文字列はクリーンとして判定されること"""
        is_clean, detected = check_compliance("")
        assert is_clean is True
        assert detected == []

    def test_ng_keywords_list_not_empty(self):
        """NGキーワードリストが空でないこと（安全装置）"""
        assert len(NG_KEYWORDS) > 0

    def test_technical_terms_not_flagged(self):
        """[正常系] 技術用語（attack vector等）はNGワードリストに含まれないこと"""
        technical_text = (
            "Adversarial attack vectors in neural networks. "
            "The model defense mechanism uses gradient masking."
        )
        is_clean, detected = check_compliance(technical_text)
        assert is_clean is True
