import os
import sys
import pytest
from fastapi.testclient import TestClient
from unittest.mock import patch, MagicMock

# Virtualize heavyweight C++ ML bound modules for offline route testing
sys.modules['rag'] = MagicMock()

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from main import app
from routes import get_current_user

# Disable authentication globally for test routes
def override_get_current_user():
    return {"username": "testadmin", "role": "admin"}

app.dependency_overrides[get_current_user] = override_get_current_user

client = TestClient(app)

@patch("routes.get_reports")
def test_dashboard_reports_api(mock_get_reports):
    mock_get_reports.return_value = {
        "total_leads": 150,
        "closed_deals": 12,
        "valid_site_punches": 45,
        "pending_internal_tasks": 3
    }
    
    response = client.get("/api/reports?org_id=1")
    assert response.status_code == 200
    data = response.json()
    assert data["total_leads"] == 150
    assert data["closed_deals"] == 12

@patch("routes.create_lead")
def test_create_lead_api(mock_create_lead):
    mock_create_lead.return_value = 99
    
    payload = {
        "first_name": "John",
        "last_name": "Doe",
        "phone": "+19999999999",
        "source": "API Test",
        "interest": "Condos",
        "org_id": 1
    }
    
    response = client.post("/api/leads", json=payload)
    assert response.status_code == 200
    assert response.json()["status"] == "success"
    assert response.json()["id"] == 99

@patch("routes.get_all_tasks")
def test_get_tasks_api(mock_get_tasks):
    mock_get_tasks.return_value = [
        {"id": 1, "department": "Legal", "description": "Review", "status": "Pending", "first_name": "John", "last_name": "Doe"}
    ]
    
    response = client.get("/api/tasks?org_id=1")
    assert response.status_code == 200
    assert len(response.json()) == 1
    assert response.json()[0]["department"] == "Legal"

@patch("routes.get_products_by_org")
def test_get_products_api(mock_get_products):
    mock_get_products.return_value = [
        {"id": 1, "name": "OceanView Condo", "org_name": "Globussoft"}
    ]
    
    response = client.get("/api/organizations/1/products")
    assert response.status_code == 200
    assert len(response.json()) == 1
    assert response.json()[0]["name"] == "OceanView Condo"

@patch("routes.update_product")
def test_update_product_api(mock_update_product):
    mock_update_product.return_value = True
    
    response = client.put("/api/products/1", json={"manual_notes": "Updated by PyTest"})
    assert response.status_code == 200
    assert response.json()["status"] == "ok"
    mock_update_product.assert_called_with(1, manual_notes="Updated by PyTest")
