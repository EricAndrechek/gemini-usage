"""FastAPI HTTP server exposing Gemini usage data."""

import asyncio
import logging

from fastapi import FastAPI
from fastapi.responses import JSONResponse

from .config import HOST, POLL_INTERVAL, PORT
from .cookies import extract_gemini_cookies
from .gemini_client import GeminiUsageClient
from .usage_counter import UsageCounter

logger = logging.getLogger(__name__)

app = FastAPI(title="Gemini Usage Tracker")
_counter: UsageCounter | None = None
_poll_task: asyncio.Task | None = None


async def _poll_loop(counter: UsageCounter) -> None:
    """Background polling loop."""
    while True:
        await counter.poll()
        await asyncio.sleep(POLL_INTERVAL)


@app.on_event("startup")
async def startup() -> None:
    global _counter, _poll_task

    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    psid, psidts = extract_gemini_cookies()
    client = GeminiUsageClient()
    await client.init(psid, psidts)

    _counter = UsageCounter(client)
    # Do an initial poll immediately
    await _counter.poll()
    # Start background polling
    _poll_task = asyncio.create_task(_poll_loop(_counter))


@app.on_event("shutdown")
async def shutdown() -> None:
    if _poll_task:
        _poll_task.cancel()


@app.get("/usage")
async def get_usage() -> JSONResponse:
    if not _counter:
        return JSONResponse({"error": "Not initialized"}, status_code=503)
    return JSONResponse(_counter.last_result.to_dict())


@app.get("/health")
async def health() -> JSONResponse:
    if not _counter:
        return JSONResponse({"status": "starting"}, status_code=503)
    result = _counter.last_result
    return JSONResponse({
        "status": "ok" if not result.error else "degraded",
        "last_updated": result.last_updated,
        "error": result.error,
    })


def run() -> None:
    import uvicorn
    uvicorn.run(app, host=HOST, port=PORT, log_level="info")
