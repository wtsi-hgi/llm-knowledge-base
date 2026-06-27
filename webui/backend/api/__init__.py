"""API package for FastAPI routers.

This module groups all API routers and is intended as the
single place where routers are imported and re-exported.
"""

from .v1 import api_router as api_v1_router

__all__ = ["api_v1_router"]
