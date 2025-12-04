"""Application configuration using pydantic-settings."""

from collections.abc import Sequence
from typing import Any

from pydantic import Field, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",
    )

    # API metadata
    app_name: str = "LLM Knowledge Base API"
    app_version: str = "0.1.0"
    app_description: str = "FastAPI backend for Next.js + shadcn/ui frontend"

    # CORS configuration
    cors_origins: list[str] = Field(
        default_factory=lambda: ["http://localhost:3000"]
    )
    cors_allow_credentials: bool = False

    # Server configuration
    backend_port: int = 8000
    host: str = "0.0.0.0"
    reload: bool = True  # Auto-reload on code changes (dev only)

    # Observability / shared resources
    log_level: str = "INFO"
    http_client_timeout: float = 10.0

    @field_validator("cors_origins", mode="before")
    @classmethod
    def parse_cors_origins(cls, value: Any) -> list[str]:
        """Ensure CORS origins always resolve to a list of strings."""

        if isinstance(value, str):
            sanitized = value.replace(";", ",")
            entries = [
                origin.strip()
                for origin in sanitized.split(",")
                if origin.strip()
            ]
            return entries or ["http://localhost:3000"]
        if isinstance(value, Sequence):
            return [str(origin).strip() for origin in value if str(origin).strip()]
        msg = "cors_origins must be a comma/semicolon separated string or list"
        raise ValueError(msg)

    @property
    def cors_origins_list(self) -> list[str]:
        """Expose parsed CORS origins for framework integrations."""

        return list(self.cors_origins)

    @model_validator(mode="after")
    def validate_cors(self) -> "Settings":
        """Prevent insecure wildcard origins when credentials are allowed."""

        if self.cors_allow_credentials and any(
            origin == "*" for origin in self.cors_origins
        ):
            msg = "cors_allow_credentials=True requires explicit origins"
            raise ValueError(msg)
        return self


# Global settings instance
settings = Settings()
