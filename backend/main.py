import os

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

app = FastAPI()

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

@app.get("/")
def read_root():
    return {"message": "Hello World from FastAPI!"}


@app.get("/hello")
def hello(name: str = "World"):
    """Example endpoint that accepts a query parameter `name` and returns a greeting."""
    return {"message": f"Hello, {name} from FastAPI!"}
