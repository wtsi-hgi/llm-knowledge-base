"""FastAPI application entry point with structured lifespan management."""

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from httpx import AsyncClient

from api import api_v1_router
from config import settings

logger = logging.getLogger("llm_kb.api")


def configure_logging() -> None:
    """Ensure loggers emit structured, leveled messages."""

    if logging.getLogger().handlers:
        # Respect host application's logging configuration (e.g., uvicorn)
        logging.getLogger().setLevel(settings.log_level.upper())
        return

    logging.basicConfig(
        level=settings.log_level.upper(),
        format="%(asctime)s | %(levelname)s | %(name)s | %(message)s",
    )


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Application lifespan context manager for startup/shutdown events."""

    configure_logging()
    logger.info(
        "Starting %s", settings.app_name, extra={"version": settings.app_version}
    )

    app.state.http_client = AsyncClient(timeout=settings.http_client_timeout)

    try:
        yield
    finally:
        http_client: AsyncClient = app.state.http_client
        await http_client.aclose()
        logger.info("Shutting down application")


app = FastAPI(
    title=settings.app_name,
    description=settings.app_description,
    version=settings.app_version,
    lifespan=lifespan,
)


# Mount versioned API router
app.include_router(api_v1_router, prefix="/api/v1")
