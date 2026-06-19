"""zatabox official Python SDK for the Zatabox Tickets REST API.

Zero third-party dependencies (urllib + hmac + hashlib). Full coverage of every
REST endpoint, generated from the canonical spec so it never drifts.

    from zatabox import Client, ZataboxError

    z = Client(api_key="vt_live_...")
    events = z.events.list({"q": "jazz", "limit": 20})
"""

from ._http import ZataboxError, __version__
from .client import Client

__all__ = ["Client", "ZataboxError", "__version__"]
