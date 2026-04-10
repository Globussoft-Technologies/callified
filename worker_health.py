"""
worker_health.py — Simple heartbeat tracker for background workers.
Workers call beat() each loop iteration; the health endpoint reads timestamps.
"""
from datetime import datetime

_heartbeats: dict[str, datetime] = {}


def beat(worker_name: str):
    """Record that a worker just completed a loop iteration."""
    _heartbeats[worker_name] = datetime.utcnow()


def get_heartbeat(worker_name: str) -> datetime | None:
    """Return the last heartbeat time for a worker, or None if never recorded."""
    return _heartbeats.get(worker_name)
