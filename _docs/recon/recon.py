"""
Reconnaissance script: dump raw Gemini API responses to understand
the data structure for usage tracking.

Extracts cookies from Brave, makes LIST_CHATS and READ_CHAT calls,
and dumps the raw JSON for analysis.
"""

import asyncio
import json
import sys

from pycookiecheat import BrowserType, get_cookies


async def main():
    # Step 1: Extract cookies from Brave
    print("=== Step 1: Extracting cookies from Brave ===")
    try:
        from pathlib import Path
        cookie_file = Path.home() / "Library/Application Support/BraveSoftware/Brave-Browser/Profile 2/Cookies"
        cookies = get_cookies(
            "https://gemini.google.com", browser=BrowserType.BRAVE, cookie_file=str(cookie_file)
        )
    except Exception as e:
        print(f"Failed to extract cookies: {e}")
        print("Make sure Brave is installed and you're logged into gemini.google.com")
        sys.exit(1)

    psid = cookies.get("__Secure-1PSID", "")
    psidts = cookies.get("__Secure-1PSIDTS", "")
    print(f"  __Secure-1PSID: {'found' if psid else 'MISSING'} ({len(psid)} chars)")
    print(f"  __Secure-1PSIDTS: {'found' if psidts else 'MISSING'} ({len(psidts)} chars)")

    if not psid:
        print("ERROR: __Secure-1PSID cookie not found. Are you logged into gemini.google.com in Brave?")
        sys.exit(1)

    # Step 2: Initialize the Gemini client
    print("\n=== Step 2: Initializing Gemini client ===")
    from gemini_webapi import GeminiClient
    from gemini_webapi.constants import GRPC
    from gemini_webapi.types import RPCData
    from gemini_webapi.utils import extract_json_from_response, get_nested_value

    client = GeminiClient(secure_1psid=psid, secure_1psidts=psidts)
    await client.init(auto_close=False, auto_refresh=False, verbose=False)
    print("  Client initialized successfully")

    # Step 3: Call LIST_CHATS and dump raw response
    print("\n=== Step 3: Calling LIST_CHATS (MaZiqc) ===")
    list_response = await client._batch_execute(
        [RPCData(rpcid=GRPC.LIST_CHATS, payload="[50,null,1]")]
    )
    print(f"  Status: {list_response.status_code}")
    print(f"  Response length: {len(list_response.text)} chars")

    # Parse the response
    response_json = extract_json_from_response(list_response.text)
    print(f"  Parsed {len(response_json)} top-level parts")

    # Dump each part
    chats_data = None
    for i, part in enumerate(response_json):
        rpcid = get_nested_value(part, [1])
        print(f"\n  Part [{i}]: rpcid={rpcid}")
        body_str = get_nested_value(part, [2])
        if body_str:
            body = json.loads(body_str)
            # Save raw body for analysis
            with open(f"recon_list_chats_part{i}.json", "w") as f:
                json.dump(body, f, indent=2, default=str)
            print(f"    Saved to recon_list_chats_part{i}.json")

            # Try to enumerate chats
            if isinstance(body, list) and len(body) > 0:
                chat_list = body[0] if isinstance(body[0], list) else None
                if chat_list and isinstance(chat_list, list):
                    print(f"    Found {len(chat_list)} items in body[0]")
                    # Show first chat's structure
                    if chat_list and isinstance(chat_list[0], list):
                        first_chat = chat_list[0]
                        print(f"    First chat has {len(first_chat)} fields")
                        # Dump the structure indices
                        for j, field in enumerate(first_chat):
                            field_str = str(field)
                            if len(field_str) > 200:
                                field_str = field_str[:200] + "..."
                            print(f"      [{j}]: {field_str}")
                        chats_data = chat_list

    # Step 4: If we found chats, read the first one to see its structure
    if chats_data and len(chats_data) > 0:
        first_chat = chats_data[0]
        # Usually [0] is the chat ID
        cid = None
        for field in first_chat:
            if isinstance(field, str) and field.startswith("c_"):
                cid = field
                break
        if not cid and isinstance(first_chat[0], str):
            cid = first_chat[0]

        if cid:
            print(f"\n=== Step 4: Calling READ_CHAT for cid={cid} ===")
            read_response = await client._batch_execute(
                [RPCData(rpcid=GRPC.READ_CHAT, payload=json.dumps([cid, 5, None, 1, [0], [4], None, 1]))]
            )
            read_json = extract_json_from_response(read_response.text)
            for i, part in enumerate(read_json):
                body_str = get_nested_value(part, [2])
                if body_str:
                    body = json.loads(body_str)
                    with open(f"recon_read_chat_part{i}.json", "w") as f:
                        json.dump(body, f, indent=2, default=str)
                    print(f"  Saved to recon_read_chat_part{i}.json")

                    # Look for model identifiers in the raw data
                    raw_str = json.dumps(body)
                    model_ids = {
                        "gemini-3.1-pro": "e6fa609c3fa255c0",
                        "gemini-3.0-flash": "fbb127bbb056c959",
                        "gemini-3.0-flash-thinking": "5bf011840784117a",
                    }
                    for name, mid in model_ids.items():
                        if mid in raw_str:
                            print(f"  Found model ID for {name}: {mid}")
                    # Also search for model name strings
                    for name in ["pro", "flash", "thinking", "gemini-3", "2.5", "3.0", "3.1"]:
                        if name.lower() in raw_str.lower():
                            # Find the context around it
                            idx = raw_str.lower().index(name.lower())
                            context = raw_str[max(0, idx-50):idx+50]
                            print(f"  Found '{name}' in response near: ...{context}...")

    # Step 5: Try the BARD_ACTIVITY RPC to see if it returns usage info
    print("\n=== Step 5: Trying BARD_ACTIVITY (ESY5D) ===")
    try:
        activity_response = await client._batch_execute(
            [RPCData(rpcid=GRPC.BARD_ACTIVITY, payload='[[["bard_activity_enabled"]]]')]
        )
        activity_json = extract_json_from_response(activity_response.text)
        for i, part in enumerate(activity_json):
            body_str = get_nested_value(part, [2])
            if body_str:
                body = json.loads(body_str)
                with open(f"recon_bard_activity_part{i}.json", "w") as f:
                    json.dump(body, f, indent=2, default=str)
                print(f"  Saved to recon_bard_activity_part{i}.json")
    except Exception as e:
        print(f"  BARD_ACTIVITY failed: {e}")

    # Step 6: Try some common RPC IDs that might return quota info
    print("\n=== Step 6: Trying potential quota/status RPCs ===")
    # These are guesses based on common Google internal RPC patterns
    potential_rpcs = [
        ("otAQ7b", "[]"),  # GET_USER_STATUS mentioned in research
        ("d8eJqb", "[]"),  # Another common one
    ]
    for rpcid, payload in potential_rpcs:
        try:
            # We need to add these to the GRPC enum or work around it
            # Since RPCData validates rpcid against GRPC enum, we'll make raw requests
            print(f"  Trying RPC '{rpcid}' with payload '{payload}'...")

            import orjson
            from httpx import Response

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
                    "f.req": orjson.dumps([[[rpcid, payload, None, "generic"]]]).decode("utf-8"),
                },
            )
            if response.status_code == 200:
                resp_json = extract_json_from_response(response.text)
                for i, part in enumerate(resp_json):
                    body_str = get_nested_value(part, [2])
                    if body_str:
                        body = json.loads(body_str)
                        fname = f"recon_rpc_{rpcid}_part{i}.json"
                        with open(fname, "w") as f:
                            json.dump(body, f, indent=2, default=str)
                        print(f"    Saved to {fname}")

                        # Look for numbers that could be quota counts
                        flat = json.dumps(body)
                        # Search for quota-like numbers
                        for keyword in ["100", "300", "limit", "quota", "usage", "remaining", "capacity"]:
                            if keyword in flat.lower():
                                idx = flat.lower().index(keyword)
                                context = flat[max(0, idx-30):idx+30]
                                print(f"    Found '{keyword}' near: {context}")
                                break
            else:
                print(f"    Status {response.status_code}")
        except Exception as e:
            print(f"    Failed: {e}")

    await client.close()
    print("\n=== Done! Check the recon_*.json files for detailed analysis ===")


if __name__ == "__main__":
    asyncio.run(main())
