"""The Zatabox client wires the generated resource namespaces onto the
standard-library transport in ``_http``."""

from ._http import Transport, ZataboxError, __version__
from . import resources


class Client(Transport):
    """Entry point for the Zatabox Tickets API.

    >>> from zatabox import Client
    >>> z = Client(api_key="vt_live_...")          # vt_test_ auto-routes to sandbox
    >>> event = z.organizer.get_event("evt_123")
    >>> for page in z.paginate(z.organizer.events, {"limit": 50}):
    ...     ...

    Pass ``api_key`` (``vt_live_`` / ``vt_test_``) or ``bearer_token`` (portal JWT /
    ``vt_mcp_`` token). Optional: ``base_url``, ``timeout``, ``max_retries``,
    ``user_agent``.
    """

    def __init__(self, api_key=None, bearer_token=None, base_url=None,
                 timeout=30.0, max_retries=2, user_agent=None):
        super().__init__(
            api_key=api_key, bearer_token=bearer_token, base_url=base_url,
            timeout=timeout, max_retries=max_retries, user_agent=user_agent,
        )
        # Attach every generated namespace: self.events, self.orders, self.organizer, …
        for name, cls in resources.RESOURCES.items():
            setattr(self, name, cls(self))
        # Hand-written extras layered onto the generated namespaces.
        self.media.upload = self.upload
        self.webhooks.verify = self.verify_webhook


__all__ = ["Client", "ZataboxError", "__version__"]
