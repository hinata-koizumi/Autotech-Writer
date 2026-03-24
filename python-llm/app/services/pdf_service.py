import logging
import httpx
import fitz  # PyMuPDF
import pymupdf4llm
from typing import Optional

logger = logging.getLogger(__name__)

class PDFService:
    def __init__(self, timeout: int = 30):
        self.timeout = timeout

    async def extract_text_from_url(self, url: str) -> Optional[str]:
        """Fetch PDF from URL and extract Markdown text."""
        logger.info(f"Extracting text from PDF URL: {url}")
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.get(url)
                resp.raise_for_status()
                content = resp.content
            
            # Use pymupdf4llm to extract Markdown
            doc = fitz.open(stream=content, filetype="pdf")
            md_text = pymupdf4llm.to_markdown(doc)
            doc.close()
            
            if not md_text:
                logger.warning(f"No text extracted from PDF at {url}")
                return None
                
            return md_text
        except Exception as e:
            logger.error(f"Failed to extract text from PDF {url}: {e}")
            return None
