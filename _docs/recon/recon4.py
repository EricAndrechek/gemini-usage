"""
Recon part 4: Verify the model ID at turn[2][4] across multiple chats.
Map each model variant to its quota bucket (Flash/Thinking/Pro).
"""

import asyncio
import json
from datetime import datetime
from pathlib import Path

from pycookiecheat import BrowserType, get_cookies


async def main():
    cookie_file = Path.home() / "Library/Application Support/BraveSoftware/Brave-Browser/Profile 2/Cookies"
    cookies = get_cookies("https://gemini.google.com", browser=BrowserType.BRAVE, cookie_file=str(cookie_file))
    psid = cookies.get("__Secure-1PSID", "")
    psidts = cookies.get("__Secure-1PSIDTS", "")

    from gemini_webapi import GeminiClient
    from gemini_webapi.constants import GRPC
    from gemini_webapi.types import RPCData
    from gemini_webapi.utils import extract_json_from_response, get_nested_value

    client = GeminiClient(secure_1psid=psid, secure_1psidts=psidts)
    await client.init(auto_close=False, auto_refresh=False, verbose=False)
    print("Client initialized")

    # Model buckets from otAQ7b response
    # Each bucket has a top-level ID and a list of sub-model variant IDs
    flash_variants = {
        "fbb127bbb056c959",  # Flash top-level
        "1bc6b5d98741cd3d", "1a43ad63cc8a7f9a", "7daceb7ef88130f5",
        "418ab5ea040b5c43", "a8236212c11d3a06", "f299729663a2343f",
        "948b866104ccf484", "cf84a01064f3134f", "35609594dbe934d8",
        "5bf011840784117a",  # Note: thinking appears in flash's sub-list
        "e051ce1aa80aa576", "7ca48d02d802f20a", "cd472a54d2abba7e",
        "f8f8f5ea629f5d37", "9c17b1863f581b8a", "1acf3172319789ce",
        "71c2d248d3b102ff", "9ec249fc9ad08861", "56fdd199312815e2",
        "797f3d0293f288ad",
    }
    thinking_variants = {
        "5bf011840784117a",  # Thinking top-level
        "1bc6b5d98741cd3d", "1a43ad63cc8a7f9a", "7daceb7ef88130f5",
        "418ab5ea040b5c43", "a8236212c11d3a06", "f299729663a2343f",
        "948b866104ccf484", "cf84a01064f3134f", "35609594dbe934d8",
        "e051ce1aa80aa576", "7ca48d02d802f20a", "cd472a54d2abba7e",
        "f8f8f5ea629f5d37", "9c17b1863f581b8a", "1acf3172319789ce",
        "71c2d248d3b102ff", "9ec249fc9ad08861", "fbb127bbb056c959",
        "56fdd199312815e2", "4af6c7f5da75d65d", "61530e79959ab139",
        "203e6bb81620bcfe", "2525e3954d185b3c",
    }
    pro_variants = {
        "9d8ca3786ebdfbea",  # Pro top-level
        "d1f674dda82d1455", "cd472a54d2abba7e", "e5a44cb1dae2b489",
        "4d79521e1e77dd3b", "b1e46a6037e6aa9f", "0e0f3a3749fc6a5c",
        "6cb69cd4b6cae77d", "e6fa609c3fa255c0", "852fc722e6249d28",
        "797f3d0293f288ad",
    }

    # Get all chats
    response = await client._batch_execute(
        [RPCData(rpcid=GRPC.LIST_CHATS, payload="[200]")]
    )
    response_json = extract_json_from_response(response.text)
    all_chats = []
    for part in response_json:
        body_str = get_nested_value(part, [2])
        if body_str and len(body_str) > 100:
            body = json.loads(body_str)
            all_chats = body[2] if len(body) > 2 and isinstance(body[2], list) else []
            break

    # Get recent chats (last few days for more data)
    recent_chats = []
    cutoff = datetime.now().timestamp() - 7 * 86400  # 7 days
    for chat in all_chats:
        if isinstance(chat, list) and len(chat) > 5:
            ts = chat[5][0] if isinstance(chat[5], list) else 0
            if ts > cutoff:
                recent_chats.append(chat)

    print(f"Found {len(recent_chats)} chats in the last 7 days")

    # Read each chat and extract model IDs from turns
    model_id_counts = {}
    chat_models = []

    for chat in recent_chats[:30]:  # Limit to 30 for speed
        cid = chat[0]
        title = chat[1]
        ts = datetime.fromtimestamp(chat[5][0])

        response = await client._batch_execute(
            [RPCData(rpcid=GRPC.READ_CHAT, payload=json.dumps([cid, 100, None, 1, [0], [4], None, 1]))]
        )
        response_json = extract_json_from_response(response.text)

        turns_data = None
        for part in response_json:
            body_str = get_nested_value(part, [2])
            if body_str and len(body_str) > 50:
                body = json.loads(body_str)
                if isinstance(body, list) and isinstance(body[0], list):
                    turns_data = body[0]
                break

        if not turns_data:
            print(f"  {cid}: No turns found")
            continue

        user_turns = 0
        model_ids_in_chat = set()
        has_thoughts = False

        for turn in turns_data:
            if not isinstance(turn, list) or len(turn) < 3:
                continue

            # Check turn[2] for user prompt section
            user_section = turn[2] if len(turn) > 2 else None
            if isinstance(user_section, list):
                # Get model ID at position [4]
                model_id = user_section[4] if len(user_section) > 4 else None
                if isinstance(model_id, str) and len(model_id) == 16:
                    model_ids_in_chat.add(model_id)
                    model_id_counts[model_id] = model_id_counts.get(model_id, 0) + 1
                user_turns += 1

            # Check for thoughts in response (thinking model indicator)
            candidates = get_nested_value(turn, [3, 0, 0])
            if candidates:
                thoughts = get_nested_value(candidates, [37, 0, 0])
                if thoughts:
                    has_thoughts = True

        # Determine bucket for each model ID
        buckets = set()
        for mid in model_ids_in_chat:
            in_flash = mid in flash_variants
            in_thinking = mid in thinking_variants
            in_pro = mid in pro_variants
            # Since flash and thinking share many variants, use exclusivity
            only_pro = in_pro and not in_flash and not in_thinking
            only_flash = in_flash and not in_pro  # might also be in thinking
            only_thinking = in_thinking and not in_pro  # might also be in flash

            if only_pro:
                buckets.add("PRO")
            elif has_thoughts:
                buckets.add("THINKING")
            elif in_flash:
                buckets.add("FLASH")
            else:
                buckets.add(f"UNKNOWN({mid})")

        bucket_str = "/".join(buckets) if buckets else "UNKNOWN"
        print(f"  {ts.strftime('%m/%d %H:%M')} {cid}: {user_turns} turns, models={model_ids_in_chat}, thoughts={has_thoughts}, bucket={bucket_str}")
        chat_models.append({
            "cid": cid,
            "title": title,
            "date": ts.isoformat(),
            "user_turns": user_turns,
            "model_ids": list(model_ids_in_chat),
            "has_thoughts": has_thoughts,
            "bucket": bucket_str,
        })

    # Summary
    print(f"\n=== Model ID frequency ===")
    for mid, count in sorted(model_id_counts.items(), key=lambda x: -x[1]):
        in_f = "F" if mid in flash_variants else " "
        in_t = "T" if mid in thinking_variants else " "
        in_p = "P" if mid in pro_variants else " "
        print(f"  {mid} [{in_f}{in_t}{in_p}]: {count} turns")

    # Save results
    with open("recon4_results.json", "w") as f:
        json.dump(chat_models, f, indent=2)
    print(f"\nSaved detailed results to recon4_results.json")

    await client.close()


if __name__ == "__main__":
    asyncio.run(main())
