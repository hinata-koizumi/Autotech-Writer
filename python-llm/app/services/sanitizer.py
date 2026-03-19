"""Payload sanitizer for X API posting."""

import re


def sanitize_for_x_api(text: str) -> str:
    """
    Sanitize text payload for X API posting.

    - Remove consecutive special symbols
    - Normalize excessive newlines
    - Remove null bytes and other control characters
    - Trim leading/trailing whitespace
    """
    # Remove null bytes
    text = text.replace("\x00", "")

    # Remove non-printable control chars (keep newlines, tabs)
    text = re.sub(r"[\x01-\x08\x0b\x0c\x0e-\x1f\x7f]", "", text)

    # Normalize excessive newlines (3+ → 2)
    text = re.sub(r"\n{3,}", "\n\n", text)

    # Remove consecutive special symbols (3+ of same symbol)
    text = re.sub(r"([!@#$%^&*()=+{}\[\]|\\:;\"'<>,./?\-_~`])\1{2,}", r"\1\1", text)

    # Trim
    text = text.strip()

    return text
