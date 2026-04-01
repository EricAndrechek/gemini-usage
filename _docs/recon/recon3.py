"""
Recon part 3: Read individual chats to find model identifiers,
and analyze chat list for field variations.
"""

import asyncio
import json
import sys
from datetime import datetime
from pathlib import Path

import orjson
from pycookiecheat import BrowserType, get_cookies


async def raw_rpc(client, rpcid, payload_str):
    from gemini_webapi.utils import extract_json_from_response, get_nested_value
    params = {
        "rpcids": rpcid,
        "_reqid": 99999,
        "rt": "c",
        "source-path": "/app",
    }
    if client.build_label:
        params["bl"] = client.build_label
    if client.session_id:
        params["f.sid"] = client.session_id

    response = await client.client.post(
        "https://gemini.google.com/_/BardChatUi/data/batchexecute",
        params=params,
        data={
            "at": client.access_token,
            "f.req": orjson.dumps([[[rpcid, payload_str, None, "generic"]]]).decode("utf-8"),
        },
    )
    return response


async def main():
    cookie_file = Path.home() / "Library/Application Support/BraveSoftware/Brave-Browser/Profile 2/Cookies"  # Work profile
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

    # Step 1: Load the 200-chat list and analyze field variations
    print("\n=== Step 1: Analyzing chat list field variations ===")
    response = await client._batch_execute(
        [RPCData(rpcid=GRPC.LIST_CHATS, payload="[200]")]
    )
    response_json = extract_json_from_response(response.text)

    all_chats = []
    for part in response_json:
        body_str = get_nested_value(part, [2])
        if body_str and len(body_str) > 100:
            body = json.loads(body_str)
            chat_list = body[2] if len(body) > 2 and isinstance(body[2], list) else []
            all_chats = chat_list
            break

    print(f"  Total chats: {len(all_chats)}")

    # Analyze field [9] (last field) variation
    field9_values = set()
    today = datetime.now().replace(hour=0, minute=0, second=0, microsecond=0)
    today_ts = today.timestamp()
    today_chats = []

    for chat in all_chats:
        if not isinstance(chat, list):
            continue
        # Check field 9
        if len(chat) > 9:
            field9_values.add(chat[9])
        # Check if today's chat
        ts = chat[5][0] if len(chat) > 5 and isinstance(chat[5], list) else 0
        dt = datetime.fromtimestamp(ts) if ts > 0 else None
        if dt and dt.date() == today.date():
            today_chats.append(chat)
        # Also check all other fields for non-null values
        for idx in range(len(chat)):
            if chat[idx] is not None and idx not in [0, 1, 5, 9]:
                pass  # Will print specific ones below

    print(f"  Field [9] values across all chats: {field9_values}")
    print(f"  Today's chats: {len(today_chats)}")

    for chat in today_chats:
        cid = chat[0]
        title = chat[1]
        ts = datetime.fromtimestamp(chat[5][0])
        f9 = chat[9] if len(chat) > 9 else "?"
        # Print all non-null fields
        non_null = {i: chat[i] for i in range(len(chat)) if chat[i] is not None}
        print(f"  {cid}: '{title}' at {ts} field9={f9}")
        print(f"    Non-null fields: {non_null}")

    # Step 2: Read a few of today's chats to find model identifiers
    print("\n=== Step 2: Reading individual chats for model info ===")
    # Read up to 3 of today's chats
    model_ids = {
        "e6fa609c3fa255c0": "gemini-3.1-pro (Pro)",
        "fbb127bbb056c959": "gemini-3.0-flash (Flash)",
        "5bf011840784117a": "gemini-3.0-flash-thinking (Thinking)",
        "9d8ca3786ebdfbea": "gemini-3-pro (Pro bucket)",
    }

    for chat in today_chats[:5]:
        cid = chat[0]
        title = chat[1]
        print(f"\n  Reading chat: {cid} - '{title}'")

        response = await client._batch_execute(
            [RPCData(rpcid=GRPC.READ_CHAT, payload=json.dumps([cid, 3, None, 1, [0], [4], None, 1]))]
        )
        response_json = extract_json_from_response(response.text)

        for i, part in enumerate(response_json):
            body_str = get_nested_value(part, [2])
            if body_str and len(body_str) > 50:
                body = json.loads(body_str)
                raw = json.dumps(body)

                # Search for model IDs
                for mid, mname in model_ids.items():
                    if mid in raw:
                        idx = raw.index(mid)
                        context = raw[max(0, idx-80):idx+100]
                        print(f"    FOUND model {mname} ({mid})")
                        print(f"    Context: ...{context}...")

                # Also dump the first turn's full structure for analysis
                turns = body[0] if isinstance(body, list) and isinstance(body[0], list) else None
                if turns and isinstance(turns[0], list):
                    first_turn = turns[0]
                    # Save the raw turn structure
                    fname = f"recon3_turn_{cid}.json"
                    with open(fname, "w") as f:
                        json.dump(first_turn, f, indent=2, default=str)
                    print(f"    Saved first turn to {fname} ({len(first_turn)} top-level fields)")

                    # Check specific indices for model info
                    for idx in range(min(len(first_turn), 50)):
                        val = first_turn[idx]
                        if val is not None:
                            val_str = str(val)
                            if len(val_str) > 300:
                                val_str = val_str[:300] + "..."
                            # Only print interesting (non-null) fields
                            if idx not in [0, 2, 3, 4]:  # Skip known fields
                                print(f"    Turn field [{idx}]: {val_str}")

    # Step 3: Try to find the model in the full raw response
    print("\n=== Step 3: Checking READ_CHAT with extended payload ===")
    if today_chats:
        cid = today_chats[0][0]
        # Try with more fields requested
        for payload_variant in [
            json.dumps([cid, 10]),
            json.dumps([cid, 10, None, 1, [0], [4], None, 1, None, None, None, 1]),
            json.dumps([cid, 10, None, 0]),
        ]:
            response = await client._batch_execute(
                [RPCData(rpcid=GRPC.READ_CHAT, payload=payload_variant)]
            )
            response_json = extract_json_from_response(response.text)
            for part in response_json:
                body_str = get_nested_value(part, [2])
                if body_str and len(body_str) > 50:
                    raw = body_str
                    found_models = [f"{mid}={mname}" for mid, mname in model_ids.items() if mid in raw]
                    print(f"  Payload {payload_variant[:60]:60s} -> {len(raw)} chars, models: {found_models}")

    # Step 4: Try some RPC IDs from the init page that look relevant
    print("\n=== Step 4: Trying RPC IDs from init page ===")
    # Read the saved RPC IDs
    try:
        with open("recon2_rpc_ids_from_init.json") as f:
            init_rpcs = json.load(f)
        print(f"  {len(init_rpcs)} candidate RPC IDs")

        # Try each one with empty payload
        for rpcid in init_rpcs:
            try:
                response = await raw_rpc(client, rpcid, "[]")
                if response.status_code == 200:
                    resp_json = extract_json_from_response(response.text)
                    for part in resp_json:
                        body_str = get_nested_value(part, [2])
                        if body_str and len(body_str) > 20:
                            body = json.loads(body_str)
                            flat = json.dumps(body).lower()
                            # Look for quota keywords
                            if any(kw in flat for kw in ["quota", "limit", "usage", "remaining", "capacity"]):
                                fname = f"recon3_rpc_{rpcid}.json"
                                with open(fname, "w") as f:
                                    json.dump(body, f, indent=2, default=str)
                                print(f"  {rpcid}: QUOTA KEYWORD FOUND! Saved to {fname}")
                            elif len(flat) < 500:
                                pass  # Small response, probably not interesting
                            else:
                                # Check for model IDs or large interesting data
                                has_model = any(mid in flat for mid in model_ids.keys())
                                if has_model:
                                    print(f"  {rpcid}: Has model IDs ({len(flat)} chars)")
            except Exception:
                pass

    except FileNotFoundError:
        print("  No init RPC IDs file found")

    await client.close()
    print("\n=== Done ===")


if __name__ == "__main__":
    asyncio.run(main())
