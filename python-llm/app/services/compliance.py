"""Compliance filter for detecting NG (political/geopolitical) keywords."""

import re
from typing import List

# Political/geopolitical NG keywords that would risk X monetization penalties
NG_KEYWORDS: List[str] = [
    "戦争", "紛争", "侵攻", "侵略", "制裁",
    "政権", "独裁", "弾圧", "虐殺", "テロ",
    "核兵器", "ミサイル", "軍事", "武装",
    "選挙不正", "クーデター", "暴動",
    "war", "invasion", "sanctions", "dictatorship",
    "genocide", "terrorism", "nuclear weapon",
    "coup", "military strike",
]


def check_compliance(text: str) -> tuple[bool, list[str]]:
    """
    Check if text contains NG keywords.

    Returns:
        (is_clean, detected_keywords): True if clean, False if NG detected.
    """
    detected = []
    lower_text = text.lower()

    for keyword in NG_KEYWORDS:
        kw_lower = keyword.lower()
        if re.search(r'[a-z]', kw_lower):
            # For English words, we want to allow matches like "これはwar関連", 
            # but prevent "war" matching "warm".
            # We enforce that if there are adjacent characters, they must not be ASCII letters.
            pattern = r'(?<![a-z])' + re.escape(kw_lower) + r'(?![a-z])'
            if re.search(pattern, lower_text):
                detected.append(keyword)
        else:
            # For Japanese words like "戦争", simple substring match is fine
            if kw_lower in lower_text:
                detected.append(keyword)

    return len(detected) == 0, detected
