"""Utilities for JSON extraction and parsing."""

import json
import logging
import re
from typing import Any, Optional

logger = logging.getLogger(__name__)


def extract_json(text: str) -> str:
    """
    Extract JSON content from a string, handling markdown and whitespace.
    Returns the raw JSON string if found, otherwise returns the original text stripped.
    """
    text = text.strip()
    
    # 1. Look for JSON block in markdown
    json_match = re.search(r"```json\s+(.*?)\s+```", text, re.DOTALL)
    if json_match:
        return json_match.group(1).strip()
    
    # 2. Look for generic code block
    code_match = re.search(r"```\s+(.*?)\s+```", text, re.DOTALL)
    if code_match:
        return code_match.group(1).strip()
    
    # 3. Find first '{' and last '}'
    start = text.find("{")
    end = text.rfind("}")
    if start != -1 and end != -1:
        return text[start:end+1].strip()
    
    return text


