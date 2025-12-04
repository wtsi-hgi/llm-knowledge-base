"""FastAPI application entry point.

This module wires together the application, configuration, middleware,
and versioned API routers. The actual endpoint implementations live in
the :mod:`api.v1` package to keep ``main.py`` focused on composition.
"""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from api import api_v1_router
from config import settings


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Application lifespan context manager for startup/shutdown events.

    Initialize and tear down shared resources here (database connections,
    model loaders, queues, etc.). Keeping this logic in a single place
    makes it easier to evolve as the app grows.
    """

    print(f"Starting {settings.app_name} v{settings.app_version}")
    yield
    print("Shutting down application")


app = FastAPI(
    title=settings.app_name,
    description=settings.app_description,
    version=settings.app_version,
    lifespan=lifespan,
)

# CORS configuration
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins_list,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# Mount versioned API router
app.include_router(api_v1_router, prefix="/api/v1")

