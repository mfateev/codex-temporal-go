# ABOUTME: One mounted external-output stream cursor for the client-side agent merge.

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator

from temporalio.client import Client
from temporalio.contrib.external_workflow_streams import (
    Cursor as ExternalCursor,
    ExternalOutputStreamClient,
    ExternalOutputStreamItem,
    Offset,
)

from temporal_agent_harness.harness.agent_protocol import (
    TURN_EVENTS_TOPIC,
    AgentEvent,
    AgentEventType,
)
from temporal_agent_harness.harness.external_streams import (
    ExternalStreamBackendFactory,
    StreamCursor,
    close_external_stream_backend,
    create_external_stream_backend,
    deserialize_stream_cursor,
    workflow_chain_key,
)

_log = logging.getLogger(__name__)


class Cursor:
    """A one-item peek-ahead reader over one agent's external output topic."""

    def __init__(
        self,
        *,
        client: Client,
        backend_factory: ExternalStreamBackendFactory,
        workflow_id: str,
        is_child: bool,
        mount_index: int,
        from_cursor: StreamCursor,
        skip_until_turn_id: str | None = None,
    ) -> None:
        self.workflow_id = workflow_id
        self.is_child = is_child
        self.mount_index = mount_index
        self._client = client
        self._backend_factory = backend_factory
        self._from_cursor = from_cursor
        self._events: AsyncIterator[ExternalOutputStreamItem[AgentEvent]] | None = None
        self._backend: object | None = None
        self._skip_until_turn_id = skip_until_turn_id
        self._skipping = skip_until_turn_id is not None

        self.head: AgentEvent | None = None
        self.head_offset: Offset | None = None
        self.head_seq = -1
        self.exhausted = False
        self.pull_task: asyncio.Task[None] | None = None
        self.error: BaseException | None = None

    @classmethod
    def mount(
        cls,
        client: Client,
        *,
        backend_factory: ExternalStreamBackendFactory = create_external_stream_backend,
        workflow_id: str,
        is_child: bool,
        mount_index: int,
        from_cursor: StreamCursor,
        skip_until_turn_id: str | None = None,
    ) -> Cursor:
        """Create a lazy external subscription positioned at ``from_cursor``."""
        return cls(
            client=client,
            backend_factory=backend_factory,
            workflow_id=workflow_id,
            is_child=is_child,
            mount_index=mount_index,
            from_cursor=from_cursor,
            skip_until_turn_id=skip_until_turn_id,
        )

    async def _connect(self) -> None:
        if self._events is not None:
            return
        backend = self._backend_factory()
        try:
            reader = await ExternalOutputStreamClient.connect(
                backend=backend,
                workflow=await workflow_chain_key(self._client, self.workflow_id),
                client=self._client,
            )
            self._events = reader.topic(TURN_EVENTS_TOPIC, type=AgentEvent).subscribe(
                after=deserialize_stream_cursor(self._from_cursor)
            )
        except BaseException:
            await close_external_stream_backend(backend)
            raise
        self._backend = backend

    @property
    def resume_cursor(self) -> StreamCursor:
        """Boundary immediately after the currently buffered/emitted head."""
        if self.head_offset is None:
            return self._from_cursor
        return ExternalCursor(self.head_offset).serialize()

    def has_reached(self, target: StreamCursor) -> bool:
        """Whether this cursor's current position is at or beyond ``target``."""
        boundary = deserialize_stream_cursor(target)
        if boundary.is_beginning:
            return True
        if self.head_offset is None or self._backend is None:
            return False
        assert boundary.offset is not None
        compare = getattr(self._backend, "compare_offsets")
        return compare(boundary.offset, self.head_offset) <= 0

    async def pull(self) -> None:
        """Buffer the next emittable event, or mark this stream exhausted/unreadable."""
        try:
            await self._connect()
            assert self._events is not None
            while True:
                item = await anext(self._events)
                ev = item.data
                if self._skipping:
                    if (
                        ev.event.type == AgentEventType.TURN_STARTED
                        and ev.turn_id == self._skip_until_turn_id
                    ):
                        self._skipping = False
                    else:
                        continue
                self.head = ev
                self.head_offset = item.offset
                return
        except StopAsyncIteration:
            self.exhausted = True
        except asyncio.CancelledError:
            raise
        except Exception as exc:  # noqa: BLE001 — child failures degrade without breaking root
            self.error = exc
            self.exhausted = True
            _log.warning(
                "stream_merge: dropping cursor for workflow %s after a read error: %r",
                self.workflow_id,
                exc,
            )

    async def aclose(self) -> None:
        """Cancel the pull, close the subscription, and release its backend."""
        if self.pull_task is not None and not self.pull_task.done():
            self.pull_task.cancel()
            try:
                await self.pull_task
            except asyncio.CancelledError:
                pass
            except Exception:  # noqa: BLE001 — teardown is best effort
                pass
        if self._events is not None:
            close = getattr(self._events, "aclose", None)
            if close is not None:
                try:
                    await close()
                except Exception:  # noqa: BLE001 — teardown is best effort
                    pass
            self._events = None
        if self._backend is not None:
            await close_external_stream_backend(self._backend)
            self._backend = None
