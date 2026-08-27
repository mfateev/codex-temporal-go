"""External Workflow Stream plumbing shared by harness clients and activities.

Workflow code never imports a provider. Workers, clients, and Activities call
``create_external_stream_backend`` (or inject an equivalent factory) so every
side binds to the same external store without serializing a live connection
through Workflow History.
"""

from __future__ import annotations

import inspect
import os
from collections.abc import AsyncIterator, Callable
from typing import Any, TypeAlias, TypeVar

from temporalio.client import Client
from temporalio.contrib.external_workflow_streams import (
    BEGINNING,
    Cursor,
    ExternalOutputStreamClient,
    ExternalOutputStreamItem,
    Offset,
    OutputStreamBackend,
    WorkflowChainKey,
)

EXTERNAL_STREAM_REDIS_URL_ENV = "TEMPORAL_AGENT_HARNESS_REDIS_URL"
EXTERNAL_STREAM_KEY_PREFIX_ENV = "TEMPORAL_AGENT_HARNESS_STREAM_KEY_PREFIX"
DEFAULT_EXTERNAL_STREAM_REDIS_URL = "redis://127.0.0.1:6379/0"
DEFAULT_EXTERNAL_STREAM_KEY_PREFIX = "temporal-agent-harness"
EXTERNAL_STREAM_REDIS_SOCKET_TIMEOUT_SECONDS = 15

StreamCursor: TypeAlias = str
"""Serialized opaque provider cursor accepted by public harness APIs."""

BEGINNING_STREAM_CURSOR: StreamCursor = BEGINNING.serialize()

ExternalStreamBackendFactory: TypeAlias = Callable[[], OutputStreamBackend]
T = TypeVar("T")


def create_external_stream_backend() -> OutputStreamBackend:
    """Create the default Redis backend from the harness environment.

    A fresh instance owns its Redis connection and may safely be closed by the
    caller. Set ``TEMPORAL_AGENT_HARNESS_REDIS_URL`` and
    ``TEMPORAL_AGENT_HARNESS_STREAM_KEY_PREFIX`` consistently on every agent
    worker and client process.
    """
    # Provider imports are forbidden in the Workflow sandbox. Keeping this
    # import inside the out-of-workflow factory lets sandbox-safe harness
    # modules refer to the factory without constructing a Redis client there.
    import redis.asyncio
    from temporalio.contrib.external_workflow_streams import RedisStreamBackend

    url = os.getenv(EXTERNAL_STREAM_REDIS_URL_ENV, DEFAULT_EXTERNAL_STREAM_REDIS_URL)
    # The provider uses a five-second blocking XREAD. redis-py's own default socket timeout is
    # also five seconds, which races the healthy empty-read response and intermittently raises a
    # storage error. Keep the transport timeout comfortably outside the provider poll interval.
    client = redis.asyncio.from_url(
        url,
        decode_responses=False,
        socket_timeout=EXTERNAL_STREAM_REDIS_SOCKET_TIMEOUT_SECONDS,
    )
    return RedisStreamBackend(
        client=client,
        key_prefix=os.getenv(
            EXTERNAL_STREAM_KEY_PREFIX_ENV, DEFAULT_EXTERNAL_STREAM_KEY_PREFIX
        ),
    )


def deserialize_stream_cursor(cursor: StreamCursor | Cursor | Offset) -> Cursor:
    """Normalize a serialized cursor (or SDK cursor/offset) for subscription."""
    if isinstance(cursor, Cursor):
        return cursor
    if isinstance(cursor, Offset):
        return Cursor(cursor)
    if not isinstance(cursor, str):
        raise TypeError(f"stream cursor must be a string, got {type(cursor).__name__}")
    return Cursor.deserialize(cursor)


async def workflow_chain_key(client: Client, workflow_id: str) -> WorkflowChainKey:
    """Resolve the stable external-stream identity for a Workflow chain."""
    description = await client.get_workflow_handle(workflow_id).describe()
    first_run_id = (
        description.raw_description.workflow_execution_info.first_run_id
    )
    if not first_run_id:
        raise RuntimeError(
            f"workflow {workflow_id!r} did not report a first execution run id"
        )
    return WorkflowChainKey(
        namespace=client.namespace,
        workflow_id=workflow_id,
        first_execution_run_id=first_run_id,
    )


async def subscribe_external_output(
    client: Client,
    workflow_id: str,
    topic: str,
    *,
    type: type[T],
    after: StreamCursor | Cursor | Offset = BEGINNING_STREAM_CURSOR,
    backend_factory: ExternalStreamBackendFactory = create_external_stream_backend,
) -> AsyncIterator[ExternalOutputStreamItem[T]]:
    """Subscribe to one typed external output topic with owned backend cleanup."""
    backend = backend_factory()
    try:
        reader = await ExternalOutputStreamClient.connect(
            backend=backend,
            workflow=await workflow_chain_key(client, workflow_id),
            client=client,
        )
        async for item in reader.topic(topic, type=type).subscribe(
            after=deserialize_stream_cursor(after)
        ):
            yield item
    finally:
        await close_external_stream_backend(backend)


async def close_external_stream_backend(backend: Any) -> None:
    """Close a factory-owned backend when it exposes a close operation."""
    close = getattr(backend, "aclose", None)
    if close is None:
        close = getattr(backend, "close", None)
    if close is None:
        return
    result = close()
    if inspect.isawaitable(result):
        await result


__all__ = [
    "BEGINNING_STREAM_CURSOR",
    "DEFAULT_EXTERNAL_STREAM_KEY_PREFIX",
    "DEFAULT_EXTERNAL_STREAM_REDIS_URL",
    "EXTERNAL_STREAM_KEY_PREFIX_ENV",
    "EXTERNAL_STREAM_REDIS_URL_ENV",
    "ExternalStreamBackendFactory",
    "StreamCursor",
    "close_external_stream_backend",
    "create_external_stream_backend",
    "deserialize_stream_cursor",
    "subscribe_external_output",
    "workflow_chain_key",
]
