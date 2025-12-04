"""Pydantic models used across the API layer."""

from pydantic import BaseModel


class MessageResponse(BaseModel):
    """Standard message response model."""

    message: str


class HealthResponse(BaseModel):
    """Health check response model."""

    status: str

