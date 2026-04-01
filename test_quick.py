"""Quick test: verify the usage counting logic works end-to-end."""

import asyncio
import sys
sys.path.insert(0, "src")

from gemini_usage.cookies import extract_gemini_cookies
from gemini_usage.gemini_client import GeminiUsageClient
from gemini_usage.usage_counter import UsageCounter, _get_reset_timestamp
from datetime import datetime


async def main():
    print("1. Extracting cookies...")
    psid, psidts = extract_gemini_cookies()
    print(f"   OK (PSID: {len(psid)} chars)")

    print("2. Initializing Gemini client...")
    client = GeminiUsageClient()
    await client.init(psid, psidts)
    print("   OK")

    print("3. Getting reset timestamp...")
    reset_ts = _get_reset_timestamp()
    print(f"   Quota reset: {datetime.fromtimestamp(reset_ts)}")

    print("4. Counting usage since reset...")
    counts = await client.count_usage_since(reset_ts)
    print(f"   Pro: {counts['pro']}")
    print(f"   Thinking: {counts['thinking']}")
    print(f"   Flash: {counts['flash']}")

    print("5. Full poll via UsageCounter...")
    counter = UsageCounter(client)
    result = await counter.poll()
    print(f"   Result: {result.to_dict()}")

    await client.close()
    print("\nDone!")


if __name__ == "__main__":
    asyncio.run(main())
