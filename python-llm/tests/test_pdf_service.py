import pytest
import respx
from httpx import Response
from app.services.pdf_service import PDFService
import fitz

@pytest.mark.asyncio
@respx.mock
async def test_extract_text_from_url_success():
    # Mock PDF content
    pdf_doc = fitz.open()
    page = pdf_doc.new_page()
    page.insert_text((50, 50), "Hello arXiv PDF!")
    pdf_bytes = pdf_doc.write()
    pdf_doc.close()

    url = "https://example.com/test.pdf"
    respx.get(url).mock(return_value=Response(200, content=pdf_bytes))

    service = PDFService()
    text = await service.extract_text_from_url(url)

    assert "Hello arXiv PDF!" in text

@pytest.mark.asyncio
@respx.mock
async def test_extract_text_from_url_failure():
    url = "https://example.com/missing.pdf"
    respx.get(url).mock(return_value=Response(404))

    service = PDFService()
    text = await service.extract_text_from_url(url)

    assert text is None
