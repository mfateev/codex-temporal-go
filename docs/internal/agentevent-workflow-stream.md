# AgentEvent history and External Workflow Streams

The harness carries its typed `AgentEvent` history on the Python SDK's experimental
`temporalio.contrib.external_workflow_streams` output feature. Event payloads live in an external
provider (Redis by default), while compact commit markers in Temporal History preserve
deterministic Workflow execution.

## The one-sentence model

Each agent publishes one typed external output topic, `turn_events`; clients consume it with an
opaque resumable cursor and merge the root agent's topic with any subagent topics referenced by
its events.

## Why external output

The previous `temporalio.contrib.workflow_streams` implementation stored the full event log in
Workflow state and moved activity publications through Signals. External output changes that
split:

- Workflow code calls `await external_output_stream.topic(...).publish(value)`. The SDK stages the
  encoded records outside Temporal and records a compact marker before making them visible.
- Activities publish directly with `ExternalOutputStreamProducer`; large model deltas do not pass
  through Signals or inflate Workflow state.
- External clients use `ExternalOutputStreamClient`. Reads remain available after the Workflow has
  completed, which makes stopped subagent histories replayable.
- Provider offsets are deliberately opaque. Public harness APIs therefore exchange serialized
  cursors such as `"B"` (beginning) and never perform integer or lexical offset arithmetic.

Queries still serve snapshots (`agent_status`, `agent_interface`). The output stream serves the
ordered history of what happened and what happens next.

## Topic and publication paths

`AgentWorkflowRunner` declares exactly one Workflow output topic:

```python
events = external_output_stream.topic("turn_events", type=AgentEvent)
await events.publish(envelope)
```

There are two producers for that same logical topic:

- Workflow-side lifecycle events are queued by the runner and published in deterministic call
  order. The runner appends a terminal finish record when it closes.
- Activity-side model/tool events use `AgentWorkflowRunner.publisher_from_activity(...)`, which
  binds an `ExternalOutputStreamProducer` to the Workflow chain carried in `TurnStreamContext`.
  Direct producers do not finish the topic; the owning Workflow does.

The context includes namespace, Workflow ID, and first execution run ID. That stable chain key
keeps output attached across retries and continue-as-new runs while preventing accidental writes
to a different Workflow that reused the same ID.

## Worker and process configuration

Every Worker that hosts an agent Workflow must receive a backend:

```python
from temporalio.worker import Worker
from temporal_agent_harness.harness import create_external_stream_backend

worker = Worker(
    client,
    task_queue="agents",
    workflows=[MyAgent],
    external_stream_backend=create_external_stream_backend(),
)
```

Clients, activity publishers, and the Nexus adapter use scoped backend instances from the same
factory. The default factory reads:

- `TEMPORAL_AGENT_HARNESS_REDIS_URL` (default `redis://127.0.0.1:6379/0`)
- `TEMPORAL_AGENT_HARNESS_STREAM_KEY_PREFIX` (default `temporal-agent-harness`)

All participating processes must use the same Redis deployment and prefix. A custom
`ExternalStreamBackendFactory` can be injected into `AgentClient`, `SubagentActivities`, activity
publishers, the merge, and the Nexus handler.

## Resume and merge semantics

`AgentClient.attach(from_offset="B", ...)` replays from the beginning. Each callback receives a
serialized root-stream resume cursor; persist it unchanged and pass it back to `attach` after a
disconnect. A cursor is a root-topic boundary, not a display ordinal, and it must never be sorted
or incremented by application code.

`send_message` snapshots the root topic tail before submitting the update, then scans forward for
the accepted turn ID. This replaces the old Workflow-returned integer acceptance offset and avoids
putting provider coordinates in Workflow state.

Subagent dispatch events carry the child's serialized cursor. The client-side gate merge mounts
that child topic at the supplied boundary and preserves the open/close brackets described in
[`unified-subagent-event-stream.md`](unified-subagent-event-stream.md).

## Durability and retention

Workflow-originated records become visible only after their corresponding History marker commits,
so a failed Workflow Task cannot leak output. Activity records are committed immediately and use
retry-stable producer identities within an Activity attempt.

External output is not automatically the same thing as indefinite retention. Redis persistence,
backup, trimming, and lifecycle policy belong to the deployment. Do not delete or trim records
that active consumers may still reference. The feature and its provider contracts are experimental
and may change with the SDK.

## See also

- [`unified-subagent-event-stream.md`](unified-subagent-event-stream.md) — multi-agent brackets
- [`human-in-the-loop-tool-approvals.md`](human-in-the-loop-tool-approvals.md) — approval events
