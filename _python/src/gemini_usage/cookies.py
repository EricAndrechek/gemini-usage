import logging

from pycookiecheat import BrowserType, get_cookies

from .config import BRAVE_COOKIE_FILE

logger = logging.getLogger(__name__)


def extract_gemini_cookies() -> tuple[str, str]:
    """Extract __Secure-1PSID and __Secure-1PSIDTS from Brave browser.

    Returns (psid, psidts) tuple. psidts may be empty string.
    Raises RuntimeError if PSID cookie is not found.
    """
    cookies = get_cookies(
        "https://gemini.google.com",
        browser=BrowserType.BRAVE,
        cookie_file=str(BRAVE_COOKIE_FILE),
    )
    psid = cookies.get("__Secure-1PSID", "")
    psidts = cookies.get("__Secure-1PSIDTS", "")

    if not psid:
        raise RuntimeError(
            f"__Secure-1PSID not found in {BRAVE_COOKIE_FILE}. "
            "Make sure you're logged into gemini.google.com in Brave."
        )

    logger.info("Extracted Gemini cookies from Brave")
    return psid, psidts
