"""Core client that talks to Gemini's internal APIs and counts prompt usage."""

import json
import logging
from datetime import datetime, timedelta

from gemini_webapi import GeminiClient
from gemini_webapi.constants import GRPC
from gemini_webapi.types import RPCData
from gemini_webapi.utils import extract_json_from_response, get_nested_value

from .config import FLASH_THINKING_SHARED_VARIANTS, PRO_ONLY_VARIANTS

logger = logging.getLogger(__name__)


def classify_turn(model_id: str | None, has_thoughts: bool) -> str:
    """Classify a turn into a quota bucket: 'pro', 'thinking', or 'flash'."""
    if model_id in PRO_ONLY_VARIANTS:
        return "pro"
    if has_thoughts:
        return "thinking"
    if model_id in FLASH_THINKING_SHARED_VARIANTS:
        return "flash"
    # Unknown model ID - assume flash (free) to avoid over-counting
    if model_id:
        logger.warning(f"Unknown model variant: {model_id}, treating as flash")
    return "flash"


class GeminiUsageClient:
    def __init__(self):
        self._client: GeminiClient | None = None

    async def init(self, psid: str, psidts: str) -> None:
        self._client = GeminiClient(secure_1psid=psid, secure_1psidts=psidts)
        await self._client.init(
            auto_close=False, auto_refresh=True, verbose=False
        )
        logger.info("Gemini client initialized")

    async def close(self) -> None:
        if self._client:
            await self._client.close()

    async def _list_chats(self, limit: int = 200) -> list[dict]:
        """Fetch the chat list and return parsed chat metadata."""
        response = await self._client._batch_execute(
            [RPCData(rpcid=GRPC.LIST_CHATS, payload=f"[{limit}]")]
        )
        response_json = extract_json_from_response(response.text)

        chats = []
        for part in response_json:
            body_str = get_nested_value(part, [2])
            if not body_str or len(body_str) < 100:
                continue
            body = json.loads(body_str)
            chat_list = body[2] if len(body) > 2 and isinstance(body[2], list) else []
            for chat in chat_list:
                if not isinstance(chat, list) or len(chat) < 6:
                    continue
                ts_data = chat[5]
                ts = ts_data[0] if isinstance(ts_data, list) and ts_data else 0
                chats.append({
                    "cid": chat[0],
                    "title": chat[1],
                    "timestamp": ts,
                })
            break

        return chats

    async def _count_turns_in_chat(self, cid: str) -> dict[str, int]:
        """Read a chat and count user turns by quota bucket."""
        counts = {"pro": 0, "thinking": 0, "flash": 0}

        try:
            response = await self._client._batch_execute(
                [RPCData(
                    rpcid=GRPC.READ_CHAT,
                    payload=json.dumps([cid, 100, None, 1, [0], [4], None, 1]),
                )]
            )
            response_json = extract_json_from_response(response.text)

            for part in response_json:
                body_str = get_nested_value(part, [2])
                if not body_str or len(body_str) < 50:
                    continue

                try:
                    body = json.loads(body_str)
                except (json.JSONDecodeError, ValueError):
                    continue

                if not isinstance(body, list) or not body or not isinstance(body[0], list):
                    continue
                turns = body[0]

                for turn in turns:
                    if not isinstance(turn, list) or len(turn) < 3:
                        continue

                    # Extract model ID from turn[2][4]
                    user_section = turn[2]
                    model_id = None
                    if isinstance(user_section, list) and len(user_section) > 4:
                        mid = user_section[4]
                        if isinstance(mid, str) and len(mid) == 16:
                            model_id = mid

                    # Check for thoughts in the response candidate
                    has_thoughts = False
                    candidates = get_nested_value(turn, [3, 0, 0])
                    if candidates:
                        thoughts = get_nested_value(candidates, [37, 0, 0])
                        if thoughts:
                            has_thoughts = True

                    bucket = classify_turn(model_id, has_thoughts)
                    counts[bucket] += 1

                break
        except Exception as e:
            logger.warning(f"Failed to read chat {cid}: {e}")

        return counts

    async def count_usage_since(self, since_ts: float) -> dict[str, int]:
        """Count all user turns by quota bucket since the given timestamp.

        Args:
            since_ts: Unix timestamp for the start of the counting period.

        Returns:
            Dict with keys 'pro', 'thinking', 'flash' and integer counts.
        """
        totals = {"pro": 0, "thinking": 0, "flash": 0}

        chats = await self._list_chats(limit=200)
        today_chats = [c for c in chats if c["timestamp"] >= since_ts]

        logger.info(f"Found {len(today_chats)} chats since {datetime.fromtimestamp(since_ts)}")

        for chat in today_chats:
            counts = await self._count_turns_in_chat(chat["cid"])
            for bucket, count in counts.items():
                totals[bucket] += count

        return totals
