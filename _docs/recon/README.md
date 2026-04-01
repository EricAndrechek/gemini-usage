# Recon Scripts

These Python scripts are used to explore Gemini's internal APIs when something
breaks or Google changes their endpoint format. They require the same Python
dependencies as the main Python prototype.

## Setup

```bash
cd python
python -m venv .venv && source .venv/bin/activate
pip install gemini-webapi pycookiecheat
```

Then run scripts from the repo root: `python docs/recon/recon.py`

## Scripts

### recon.py — Basic API Probing
First thing to run when debugging. Tests authentication, makes LIST_CHATS and
READ_CHAT calls, and tries the `otAQ7b` (GET_USER_STATUS) RPC to dump available
model definitions. Outputs JSON files for each response.

**When to use**: Auth broken, or need to see what the API returns at all.

### recon2.py — LIST_CHATS Payload Testing
Tests different payload formats for the LIST_CHATS RPC to find which ones return
data. Also scrapes the Gemini init page for RPC IDs and tries each one looking
for quota-related endpoints.

**When to use**: LIST_CHATS stops returning data, or searching for new RPCs.

### recon3.py — READ_CHAT Structure Analysis
Reads multiple chats and dumps the raw turn structure field by field. Looks for
model IDs at various indices and checks for the `thoughts` field in different
positions.

**When to use**: Model ID or thoughts field has moved to a different index.

### recon4.py — Model ID Extraction & Verification
The most important recon script. Reads ~30 recent chats and extracts the model
variant ID from `turn[2][4]`, cross-references with the model bucket lists from
`otAQ7b`, and uses the `thoughts` heuristic to classify each chat.

**When to use**: Need to rebuild the model variant → quota bucket mapping (e.g.,
after Google adds new model variants or changes IDs).

## Important Notes

- All scripts use `BraveSoftware/Brave-Browser/Profile 2/Cookies` by default.
  Change the `cookie_file` path if using a different profile.
- Output JSON files are gitignored. The scripts write `recon*.json` files to
  the current working directory.
- These scripts make real API calls to Google's servers. Don't run them in a loop.
