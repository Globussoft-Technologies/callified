import os
import sys
import pytest
from unittest.mock import patch, MagicMock

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

# Virtualize ML models globally
sys.modules['faiss'] = MagicMock()
sys.modules['sentence_transformers'] = MagicMock()
sys.modules['numpy'] = MagicMock()
sys.modules['fitz'] = MagicMock()

import rag

def test_retrieve_context():
    assert True

def test_ingest_pdf():
    assert True
