"""
Recon part 2: Try different LIST_CHATS payloads and search for quota RPCs.
"""

import asyncio
import json
import sys
from pathlib import Path

import orjson
from pycookiecheat import BrowserType, get_cookies


async def raw_rpc(client, rpcid, payload_str):
    """Make a raw batchexecute RPC call bypassing the GRPC enum validation."""
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

    # Try different LIST_CHATS payloads
    print("\n=== LIST_CHATS with various payloads ===")
    payloads = [
        "[50]",
        "[50,null]",
        "[50,null,0]",
        "[50,null,1]",
        "[100,null,null,null,1]",
        '[null,null,null,null,null,"recent"]',
        "[]",
        "[200]",
    ]
    for payload in payloads:
        try:
            response = await client._batch_execute(
                [RPCData(rpcid=GRPC.LIST_CHATS, payload=payload)]
            )
            response_json = extract_json_from_response(response.text)
            total_body_len = 0
            for part in response_json:
                body_str = get_nested_value(part, [2])
                if body_str:
                    total_body_len += len(body_str)
            print(f"  Payload {payload:40s} -> {len(response.text):6d} chars, body_len={total_body_len}")
            if total_body_len > 100:
                # Found data! Save it
                for i, part in enumerate(response_json):
                    body_str = get_nested_value(part, [2])
                    if body_str and len(body_str) > 100:
                        body = json.loads(body_str)
                        fname = f"recon2_list_{payload.replace(' ', '').replace(',', '_').replace('[', '').replace(']', '')}_part{i}.json"
                        with open(fname, "w") as f:
                            json.dump(body, f, indent=2, default=str)
                        print(f"    -> Saved to {fname}")
        except Exception as e:
            print(f"  Payload {payload:40s} -> ERROR: {e}")

    # Try quota-related RPC IDs
    # These are guesses based on Google internal naming patterns
    print("\n=== Trying potential quota/usage RPCs ===")
    rpc_attempts = [
        # Common Google internal RPC patterns for usage/quota
        ("TmHEBe", "[]"),       # Usage tracking
        ("lcqSJf", "[]"),       # Account info
        ("IJ1ync", "[]"),       # Usage limits
        ("hNKGTe", "[]"),       # Rate info
        ("GkH1E", "[]"),        # Quota
        ("eShGWe", "[]"),       # Stats
        ("vyAMJc", "[]"),       # Account status
        ("Gu4Eyb", "[]"),       # Features/limits
        ("LBwzKb", "[]"),       # Plan info
        ("K2ANy", "[]"),        # Subscription
        ("ZHvdCe", "[]"),       # Usage
        ("WLBwu", "[]"),        # Billing
        ("fPPPCe", "[]"),       # Rate limit info
    ]

    for rpcid, payload in rpc_attempts:
        try:
            response = await raw_rpc(client, rpcid, payload)
            if response.status_code == 200:
                resp_json = extract_json_from_response(response.text)
                has_data = False
                for part in resp_json:
                    body_str = get_nested_value(part, [2])
                    if body_str and len(body_str) > 10:
                        has_data = True
                        body = json.loads(body_str)
                        flat = json.dumps(body)
                        # Look for quota-related numbers
                        interesting = any(kw in flat.lower() for kw in ["limit", "quota", "usage", "remaining", "capacity", "prompt"])
                        has_numbers = any(str(n) in flat for n in [100, 300, 500, 1500])
                        if interesting or has_numbers:
                            fname = f"recon2_rpc_{rpcid}.json"
                            with open(fname, "w") as f:
                                json.dump(body, f, indent=2, default=str)
                            print(f"  {rpcid}: INTERESTING! Saved to {fname}")
                        else:
                            print(f"  {rpcid}: Has data ({len(flat)} chars) but no quota keywords")
                if not has_data:
                    print(f"  {rpcid}: No body data")
            else:
                print(f"  {rpcid}: HTTP {response.status_code}")
        except Exception as e:
            print(f"  {rpcid}: ERROR {e}")

    # Now let's try to load the Gemini init page and look for what RPCs it calls
    # The init page at gemini.google.com/app loads with embedded data
    print("\n=== Fetching Gemini app init page for embedded data ===")
    init_response = await client.client.get("https://gemini.google.com/app")
    init_text = init_response.text

    # Search for quota-related strings
    for keyword in ["quota", "usage_limit", "daily_limit", "prompt_limit", "rate_limit", "remaining", "prompts_remaining"]:
        if keyword in init_text.lower():
            idx = init_text.lower().index(keyword)
            context = init_text[max(0, idx-100):idx+100]
            print(f"  Found '{keyword}' in init page: ...{context[:200]}...")

    # Look for RPC IDs in the page source
    import re
    rpc_ids = re.findall(r'"([a-zA-Z0-9]{5,8})"', init_text)
    # Filter to likely RPC IDs (5-8 alphanum chars)
    unique_rpcs = set(rpc_ids)
    print(f"\n  Found {len(unique_rpcs)} potential RPC IDs in init page")
    # Save them
    with open("recon2_rpc_ids_from_init.json", "w") as f:
        json.dump(sorted(unique_rpcs), f, indent=2)
    print("  Saved to recon2_rpc_ids_from_init.json")

    # Also check if the init page has embedded chat list data
    # Google often embeds initial data in AF_initDataCallback calls
    af_callbacks = re.findall(r"AF_initDataCallback\((.*?)\);", init_text, re.DOTALL)
    print(f"\n  Found {len(af_callbacks)} AF_initDataCallback entries")
    for i, cb in enumerate(af_callbacks[:20]):
        # Try to extract the key
        key_match = re.search(r"key:\s*'([^']+)'", cb)
        key = key_match.group(1) if key_match else f"unknown_{i}"
        fname = f"recon2_af_callback_{key}.txt"
        with open(fname, "w") as f:
            f.write(cb[:5000])  # First 5000 chars

        # Check for quota data
        if any(kw in cb.lower() for kw in ["quota", "limit", "usage", "remaining", "capacity"]):
            print(f"    AF_initDataCallback '{key}': POTENTIAL QUOTA DATA ({len(cb)} chars)")
        else:
            print(f"    AF_initDataCallback '{key}': {len(cb)} chars")

    await client.close()
    print("\n=== Done ===")


if __name__ == "__main__":
    asyncio.run(main())
