import os
from contextlib import asynccontextmanager
from typing import AsyncIterator

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Application lifespan context manager for startup/shutdown events."""
    # Startup: Initialize resources here (database connections, ML models, etc.)
    yield
    # Shutdown: Clean up resources here


app = FastAPI(
    title="LLM Knowledge Base API",
    description="FastAPI backend for the Next.js + shadcn/ui frontend",
    version="0.1.0",
    lifespan=lifespan,
)

# CORS configuration
# In production, replace "*" with specific origins like ["https://yourdomain.com"]
CORS_ORIGINS = os.getenv("CORS_ORIGINS", "*").split(",")

app.add_middleware(
    CORSMiddleware,
    allow_origins=CORS_ORIGINS,
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
    """Greeting endpoint that accepts a query parameter `name` and returns a greeting."""
    return MessageResponse(message=f"Hello, {name} from FastAPI!")
