# Compliance filter for detecting NG (political/geopolitical) keywords.
import re

NG_KEYWORDS: list[str] = [
    "戦争", "紛争", "侵攻", "侵略", "制裁",
    "政権", "独裁", "弾圧", "虐殺", "テロ",
    "核兵器", "ミサイル", "軍事", "武装",
    "選挙不正", "クーデター", "暴動",
    "war", "invasion", "sanctions", "dictatorship",
    "genocide", "terrorism", "nuclear weapon",
    "coup", "military strike",
]

# Pre-compiled regex for English NG keywords (word-boundary match, case-insensitive).
_ENGLISH_KEYWORDS = [kw for kw in NG_KEYWORDS if re.search(r'[a-z]', kw)]
_JAPANESE_KEYWORDS = [kw for kw in NG_KEYWORDS if not re.search(r'[a-z]', kw)]
_ENGLISH_NG_RE = re.compile(
    r'(?<![a-z])(' + '|'.join(re.escape(kw) for kw in _ENGLISH_KEYWORDS) + r')(?![a-z])',
    re.IGNORECASE,
)


def check_compliance(text: str, ng_keywords: list[str] = NG_KEYWORDS) -> tuple[bool, list[str]]:
    """
    Check if text contains NG keywords.

    Returns:
        (is_clean, detected_keywords): True if clean, False if NG detected.
    """
    detected = []
    lower_text = text.lower()

    # Use pre-compiled regex when called with the default keyword list for efficiency.
    if ng_keywords is NG_KEYWORDS:
        for kw in _JAPANESE_KEYWORDS:
            if kw in lower_text:
                detected.append(kw)
        for m in _ENGLISH_NG_RE.finditer(lower_text):
            detected.append(m.group(0))
    else:
        for keyword in ng_keywords:
            kw_lower = keyword.lower()
            if re.search(r'[a-z]', kw_lower):
                pattern = r'(?<![a-z])' + re.escape(kw_lower) + r'(?![a-z])'
                if re.search(pattern, lower_text):
                    detected.append(keyword)
            else:
                if kw_lower in lower_text:
                    detected.append(keyword)

    return len(detected) == 0, detected
