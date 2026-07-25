FROM python:3.13-slim

COPY --from=ghcr.io/astral-sh/uv:0.11.32 /uv /uvx /bin/

WORKDIR /app
COPY pyproject.toml uv.lock handler.py ./
RUN uv sync --locked --no-dev

ENV PATH="/app/.venv/bin:${PATH}"
ENTRYPOINT ["python", "handler.py"]
