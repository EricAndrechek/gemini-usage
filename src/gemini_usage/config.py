from pathlib import Path

# Brave cookie file path (Profile 2 = work profile)
BRAVE_COOKIE_FILE = (
    Path.home()
    / "Library/Application Support/BraveSoftware/Brave-Browser/Profile 2/Cookies"
)

# Quota limits for Workspace Business Standard
PRO_LIMIT = 100
THINKING_LIMIT = 300

# Polling interval in seconds
POLL_INTERVAL = 60

# HTTP server
HOST = "0.0.0.0"
PORT = 8420

# Model variant -> quota bucket mapping
# From otAQ7b response: each bucket has a top-level ID and sub-variant IDs.
# Pro variants are exclusive to the Pro bucket.
# Flash and Thinking share many variant IDs, so we use the `thoughts` field
# in the response to distinguish them.
PRO_ONLY_VARIANTS = {
    "9d8ca3786ebdfbea",  # Pro bucket top-level
    "d1f674dda82d1455",
    "e5a44cb1dae2b489",
    "4d79521e1e77dd3b",
    "b1e46a6037e6aa9f",
    "0e0f3a3749fc6a5c",
    "6cb69cd4b6cae77d",
    "e6fa609c3fa255c0",  # gemini-3.1-pro (most common)
    "852fc722e6249d28",
}

# Variants that appear in BOTH Flash and Thinking buckets.
# If thoughts are present -> Thinking quota. Otherwise -> Flash (free).
FLASH_THINKING_SHARED_VARIANTS = {
    "fbb127bbb056c959",  # Flash top-level
    "5bf011840784117a",  # Thinking top-level
    "1bc6b5d98741cd3d",
    "1a43ad63cc8a7f9a",
    "7daceb7ef88130f5",
    "418ab5ea040b5c43",
    "a8236212c11d3a06",
    "f299729663a2343f",
    "948b866104ccf484",
    "cf84a01064f3134f",
    "35609594dbe934d8",
    "e051ce1aa80aa576",
    "7ca48d02d802f20a",
    "cd472a54d2abba7e",
    "f8f8f5ea629f5d37",
    "9c17b1863f581b8a",
    "1acf3172319789ce",
    "71c2d248d3b102ff",
    "9ec249fc9ad08861",
    "56fdd199312815e2",
    "797f3d0293f288ad",
    "4af6c7f5da75d65d",
    "61530e79959ab139",
    "203e6bb81620bcfe",
    "2525e3954d185b3c",
}
