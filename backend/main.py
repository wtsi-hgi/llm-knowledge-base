"""FastAPI application entry point."""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

from config import settings


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Application lifespan context manager for startup/shutdown events."""
    # Startup: Initialize resources here (database connections, ML models, etc.)
    print(f"Starting {settings.app_name} v{settings.app_version}")
    yield
    # Shutdown: Clean up resources here
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


class MessageResponse(BaseModel):
    """Standard message response model."""

    message: str


class HealthResponse(BaseModel):
    """Health check response model."""

    status: str


@app.get("/health", response_model=HealthResponse, tags=["health"])
async def health_check() -> HealthResponse:
    """Health check endpoint for monitoring and load balancers."""
    return HealthResponse(status="healthy")


@app.get("/", response_model=MessageResponse, tags=["root"])
async def read_root() -> MessageResponse:
    """Root endpoint returning a welcome message."""
    return MessageResponse(message="Hello World from FastAPI!")


@app.get("/hello", response_model=MessageResponse, tags=["greetings"])
async def hello(name: str = "World") -> MessageResponse:
    """Greeting endpoint that accepts a query parameter `name`."""
    return MessageResponse(message=f"Hello, {name} from FastAPI!")
