"""Caching layer that polls Gemini and tracks quota usage."""

import logging
from dataclasses import dataclass, field
from datetime import datetime, timezone

from .config import PRO_LIMIT, THINKING_LIMIT
from .gemini_client import GeminiUsageClient

logger = logging.getLogger(__name__)


@dataclass
class QuotaInfo:
    used: int = 0
    limit: int = 0

    @property
    def remaining(self) -> int:
        return max(0, self.limit - self.used)

    def to_dict(self) -> dict:
        return {"used": self.used, "limit": self.limit, "remaining": self.remaining}


@dataclass
class UsageResult:
    pro: QuotaInfo = field(default_factory=lambda: QuotaInfo(limit=PRO_LIMIT))
    thinking: QuotaInfo = field(default_factory=lambda: QuotaInfo(limit=THINKING_LIMIT))
    flash_count: int = 0
    last_updated: str = ""
    error: str | None = None

    def to_dict(self) -> dict:
        return {
            "pro": self.pro.to_dict(),
            "thinking": self.thinking.to_dict(),
            "flash": self.flash_count,
            "last_updated": self.last_updated,
            "error": self.error,
        }


def _get_reset_timestamp() -> float:
    """Get the Unix timestamp for the start of the current quota period.

    Gemini quota resets at midnight Pacific Time.
    """
    import zoneinfo
    pacific = zoneinfo.ZoneInfo("America/Los_Angeles")
    now = datetime.now(pacific)
    midnight = now.replace(hour=0, minute=0, second=0, microsecond=0)
    return midnight.timestamp()


class UsageCounter:
    def __init__(self, client: GeminiUsageClient):
        self._client = client
        self._last_result = UsageResult()

    @property
    def last_result(self) -> UsageResult:
        return self._last_result

    async def poll(self) -> UsageResult:
        """Fetch current usage from Gemini and return updated result."""
        try:
            since_ts = _get_reset_timestamp()
            counts = await self._client.count_usage_since(since_ts)

            result = UsageResult(
                pro=QuotaInfo(used=counts["pro"], limit=PRO_LIMIT),
                thinking=QuotaInfo(used=counts["thinking"], limit=THINKING_LIMIT),
                flash_count=counts["flash"],
                last_updated=datetime.now(timezone.utc).isoformat(),
            )
            self._last_result = result
            logger.info(
                f"Usage: pro={counts['pro']}/{PRO_LIMIT}, "
                f"thinking={counts['thinking']}/{THINKING_LIMIT}, "
                f"flash={counts['flash']}"
            )
            return result

        except Exception as e:
            logger.error(f"Poll failed: {e}")
            self._last_result.error = str(e)
            self._last_result.last_updated = datetime.now(timezone.utc).isoformat()
            return self._last_result
