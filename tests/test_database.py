import os
import sys
import pytest
from unittest.mock import MagicMock, patch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

import database

@pytest.fixture
def mock_db():
    with patch("database.get_conn") as mock_get_conn:
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        
        # Configure standard cursor fetch behaviors to return empty dicts or lists
        mock_cursor.fetchone.return_value = {"cnt": 0, "status": "new", "id": 1, "name": "Fake Product", "test": "val"}
        mock_cursor.fetchall.return_value = [{"id": 1, "name": "Test Org"}]
        mock_cursor.rowcount = 1
        mock_cursor.lastrowid = 1
        
        mock_conn.cursor.return_value = mock_cursor
        mock_get_conn.return_value = mock_conn
        
        yield mock_cursor

def test_database_initialization(mock_db):
    """Verify that init_db executes all table creation schemas natively without syntax errors."""
    database.init_db()
    
    # Collect all executed SQL command strings
    queries = [call[0][0] for call in mock_db.execute.call_args_list]
    
    assert any("CREATE TABLE IF NOT EXISTS leads" in q for q in queries)
    assert any("CREATE TABLE IF NOT EXISTS calls" in q for q in queries)
    assert any("CREATE TABLE IF NOT EXISTS call_transcripts" in q for q in queries)
    assert any("CREATE TABLE IF NOT EXISTS crm_integrations" in q for q in queries)

def test_create_organization(mock_db):
    org_id = database.create_organization("Globussoft Demo")
    assert org_id == 1
    mock_db.execute.assert_called_with("INSERT INTO organizations (name) VALUES (%s)", ("Globussoft Demo",))

def test_create_and_fetch_lead(mock_db):
    lead_id = database.create_lead({"first_name": "Test", "phone": "+9100"}, org_id=1)
    assert lead_id == 1
    
    database.get_all_leads(1)
    mock_db.execute.assert_called_with("SELECT * FROM leads WHERE org_id = %s ORDER BY id DESC", (1,))

def test_call_status_updater(mock_db):
    database.log_call_status("+91000", "completed", "No errors")
    
    # Assert the final update query was fired
    args = mock_db.execute.call_args[0]
    assert "UPDATE leads" in args[0]
    assert "SET status = %s" in args[0]
    assert args[1][0] == "Calling..."  # completed triggers Calling... logically
    assert "[20" in args[1][1] # Timestamp exists in follow_up_note
    
def test_save_call_transcript(mock_db):
    transcript_json = '{"text": "Hello, how are you?"}'
    database.save_call_transcript(1, transcript_json, "https://exotel.recording.mp3", 45.5)
    
    mock_db.execute.assert_called_with(
        "INSERT INTO call_transcripts (lead_id, transcript, recording_url, call_duration_s) VALUES (%s, %s, %s, %s)",
        (1, transcript_json, "https://exotel.recording.mp3", 45.5)
    )

def test_crm_integration_flow(mock_db):
    database.save_crm_integration("hubspot", {"token": "123"}, org_id=1)
    
    # Assert SELECT checking for existing
    assert "SELECT id FROM crm_integrations" in mock_db.execute.call_args_list[0][0][0]
    # Assert UPDATE into integration table
    assert "UPDATE crm_integrations" in mock_db.execute.call_args_list[1][0][0]
