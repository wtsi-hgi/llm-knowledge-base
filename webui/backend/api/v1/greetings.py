"""Greeting-related endpoints."""

from typing import Annotated

from fastapi import APIRouter, Query

from ..schemas import MessageResponse

router = APIRouter()


@router.get("/", response_model=MessageResponse)
async def read_root() -> MessageResponse:
    """Root endpoint returning a welcome message."""

    return MessageResponse(message="Hello World from FastAPI!")


@router.get("/hello", response_model=MessageResponse)
async def hello(
    name: Annotated[str, Query(description="Name to greet")] = "World",
) -> MessageResponse:
    """Greeting endpoint that accepts a query parameter `name`."""

    return MessageResponse(message=f"Hello, {name} from FastAPI!")
