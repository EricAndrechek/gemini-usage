# Gemini Web App Internal API Reference

> This document captures reverse-engineered details of gemini.google.com's
> internal APIs as of April 2026. Use this to understand how the usage tracker
> works and to fix things when Google inevitably changes endpoints or data formats.

## Authentication

### Cookies Required

Gemini uses standard Google session cookies for authentication:

- **`__Secure-1PSID`** (required): Primary session cookie. Long-lived (months).
- **`__Secure-1PSIDTS`** (optional but recommended): Timestamp/refresh token.
  Rotates frequently. The `gemini-webapi` Python library has a built-in
  `RotateCookies` mechanism at `https://accounts.google.com/RotateCookies`.

These cookies come from the `.google.com` domain in your browser's cookie store.
On macOS, Brave stores them in an encrypted SQLite database at:
```
~/Library/Application Support/BraveSoftware/Brave-Browser/<Profile>/Cookies
```

The encryption key is in the macOS Keychain under "Brave Safe Storage".

### Access Token (SNlM0e)

Before making API calls, you must fetch `https://gemini.google.com/app` with
valid cookies. The HTML response contains embedded tokens:

```
"SNlM0e":"<access_token>"    — Required for all batchexecute calls (the `at` param)
"cfb2h":"<build_label>"      — Build label, sent as `bl` query param
"FdrFJe":"<session_id>"      — Session ID, sent as `f.sid` query param
```

Extract these with regex. They change with each Gemini deployment.

## batchexecute Endpoint

**URL**: `https://gemini.google.com/_/BardChatUi/data/batchexecute`

**Method**: POST

**Query Parameters**:
| Param | Value |
|-------|-------|
| `rpcids` | Comma-separated RPC IDs (e.g., `MaZiqc`) |
| `_reqid` | Request counter (increment by 100000) |
| `rt` | `c` |
| `source-path` | `/app` |
| `bl` | Build label from SNlM0e extraction |
| `f.sid` | Session ID from SNlM0e extraction |

**Form Data**:
| Field | Value |
|-------|-------|
| `at` | Access token (SNlM0e value) |
| `f.req` | JSON payload: `[[["{rpcid}","{payload_as_string}",null,"generic"]]]` |

**CRITICAL**: The payload inside `f.req` must be a JSON **string**, not raw JSON.
For example, to call `MaZiqc` with payload `[50]`, the `f.req` value is:
```json
[[["MaZiqc","[50]",null,"generic"]]]
```
Note `"[50]"` is a string, not the array `[50]`.

### Response Format

Responses use Google's length-prefixed framing protocol:

1. Optional anti-XSSI prefix: `)]}'\n`
2. One or more frames, each formatted as: `{length}\n{json_content}`
3. Length is in **UTF-16 code units** (matching JavaScript `String.length`)

Each frame contains a JSON array. After parsing, response parts look like:
```json
["{rpcid}", null, "{inner_json_string}", ...]
```
The actual data is at index `[2]` as a JSON-encoded string that must be parsed again.

## RPC Endpoints

### MaZiqc — LIST_CHATS

**Payload**: `[{limit}]` (e.g., `[200]`)

**Response structure** (after parsing body string at index [2]):
```
body[2] = array of chats, each:
  [0]: chat ID (string, e.g., "c_77ab2f6b9faa3039")
  [1]: title (string)
  [2]: null
  [3]: null
  [4]: null
  [5]: [epoch_seconds, nanoseconds]
  [6]: null
  [7]: null
  [8]: null
  [9]: 3 (constant, purpose unknown — possibly Gemini version)
```

**Notes**:
- Payload `[50,null,0]` or `[50,null,1]` returns ~empty responses. Use `[50]` or `[200]`.
- Chat list does NOT contain model information. You must read each chat individually.
- Chats from all devices (desktop, phone app) appear in the same list.

### hNvQHb — READ_CHAT

**Payload**: `["{cid}", {max_turns}, null, 1, [0], [4], null, 1]`

**Response structure** (body[0] = array of turns, each turn):
```
turn[0]: [cid, rid] — metadata
turn[1]: [cid, rid, rcid] — extended metadata
turn[2]: user prompt section
  [0]: [prompt_text]
  [1]: number (observed: 3)
  [2]: null
  [3]: number (observed: 0)
  [4]: MODEL_VARIANT_ID (16-char hex string) ← KEY FIELD
  [5]: number
  ...remaining fields...
turn[3]: response section
  [0]: array of candidates
    [0]: candidate data
      [0]: rcid
      [1]: [response_text]
      [2]: source citations
      ...
      [37]: thoughts section (THINKING MODELS ONLY)
        [0]: [thought_text]
turn[4]: [epoch_seconds, nanoseconds]
```

**The model variant ID at `turn[2][4]` is the critical field** for determining
which quota bucket a prompt counts against.

### otAQ7b — GET_USER_STATUS

**Payload**: `[]`

Returns account information including available models. Does NOT return
quota usage counts. Response at index [15] contains model definitions:

```json
[
  ["fbb127bbb056c959", "Fast", "Answers quickly", [...capabilities...], ...],
  ["5bf011840784117a", "Thinking", "Solves complex problems", [...], ...],
  ["9d8ca3786ebdfbea", "Pro", "Advanced math and code with 3.1 Pro", [...], ...]
]
```

Each model entry contains a list of variant IDs at index [6] that belong to that
model's "bucket". This is how we know which model IDs map to which quota bucket.

### ESY5D — BARD_ACTIVITY

**Payload**: `[[["bard_activity_enabled"]]]`

Returns `[[null,null,null,null,true]]`. Used internally before file uploads and
content generation. Not useful for usage tracking.

## Model Variant ID → Quota Bucket Mapping

As of April 2026, three quota buckets exist:

### Pro (counts against 100/day limit)
These variant IDs appear ONLY in the Pro model's sub-ID list:
```
9d8ca3786ebdfbea  — Pro bucket top-level ID
d1f674dda82d1455
e5a44cb1dae2b489
4d79521e1e77dd3b
b1e46a6037e6aa9f
0e0f3a3749fc6a5c
6cb69cd4b6cae77d
e6fa609c3fa255c0  — gemini-3.1-pro (most commonly observed)
852fc722e6249d28
```

### Flash (free, no quota) and Thinking (counts against 300/day limit)
These variant IDs are SHARED between Flash and Thinking buckets:
```
fbb127bbb056c959  — Flash top-level ID
5bf011840784117a  — Thinking top-level ID
56fdd199312815e2  — frequently observed for Flash
e051ce1aa80aa576  — frequently observed for Thinking
(~20 more shared IDs, see config.go for full list)
```

**To distinguish Flash from Thinking**: Check if the response candidate has
a non-empty `thoughts` field at `turn[3][0][0][37][0][0]`.
- Has thoughts → **Thinking** (counts against 300/day quota)
- No thoughts → **Flash** (free, unlimited)

### Classification Algorithm
```
if model_id in PRO_ONLY_VARIANTS:
    return "pro"
if response has thoughts:
    return "thinking"
return "flash"
```

## Quota Details

- **Workspace Business Standard**: 100 Pro + 300 Thinking per day
- **Reset time**: Midnight Pacific Time (America/Los_Angeles)
- **No official quota API**: Google does not expose remaining quota programmatically.
  The only way to know usage is to count conversation turns yourself.
- Google shows a warning when "close" to the limit, and a hard stop at the limit.
  These warnings appear to be client-side, triggered by response error code 1037
  (`USAGE_LIMIT_EXCEEDED`).

## Known Error Codes (from gemini-webapi)

| Code | Name | Meaning |
|------|------|---------|
| 1013 | TEMPORARY_ERROR | Random transient error, retry usually works |
| 1037 | USAGE_LIMIT_EXCEEDED | Hit the daily quota — this IS the data |
| 1050 | MODEL_INCONSISTENT | Model mismatch in conversation |
| 1052 | MODEL_HEADER_INVALID | Bad model header format |
| 1060 | IP_TEMPORARILY_BLOCKED | Rate-limited by IP |

## What Google Could Change (and How to Fix It)

1. **Model variant IDs change**: Run `recon.py` or call `otAQ7b` to get the
   current model list with variant IDs. Update `config.go` / `config.py`.

2. **RPC IDs change**: The `MaZiqc`, `hNvQHb`, `otAQ7b` IDs could change with
   a Gemini frontend deployment. Run `recon2.py` step 6 to scrape RPC IDs from
   the init page. Test each one to find the new list/read chat RPCs.

3. **Response structure changes**: Field positions might shift. Run `recon3.py`
   to dump raw turn structures and find where model IDs and thoughts moved to.

4. **batchexecute format changes**: Unlikely (it's a Google-wide protocol) but
   if responses stop parsing, check if the length-prefixed framing or anti-XSSI
   prefix format changed.

5. **New quota tiers**: If Google changes limits, update `ProLimit` and
   `ThinkingLimit` in `config.go` / `config.py`.

6. **Thoughts field moves**: If the thinking model indicator moves from index
   `[37][0][0]` to somewhere else, run `recon4.py` and look for the `has_thoughts`
   detection to find where it went.
