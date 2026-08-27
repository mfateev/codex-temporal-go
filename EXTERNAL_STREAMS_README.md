# External Workflow Streams fork and branch setup

External Workflow Streams is not released in the Temporal Python SDK yet. The harness therefore
uses development branches in Maxim Fateev's forks and pins the Python SDK to an exact commit.
This guide explains how to install, reproduce, run, and update that stack.

## Repository topology

| Component | Repository | Development branch |
|---|---|---|
| Agent harness | `https://github.com/mfateev/temporal-agent-harness.git` | `task/external-workflow-streams` |
| Temporal Python SDK | `https://github.com/mfateev/sdk-python.git` | `task/python-sdk-streaming` |
| Temporal SDK Core | `https://github.com/mfateev/sdk-core.git` | `task/python-sdk-streaming` |

The versions are selected at three different levels:

1. An application selects the harness branch or, preferably, an exact harness commit.
2. The harness's `pyproject.toml` selects an exact `sdk-python` commit. `uv.lock` records the same
   source revision. This pin is the source of truth for the SDK used by the harness.
3. `sdk-python` records the exact SDK Core commit through its
   `temporalio/bridge/sdk-core` submodule. Its `.gitmodules` file points that submodule at the
   `mfateev/sdk-core` fork and names `task/python-sdk-streaming` as its development branch.

Following a branch is useful while developing. Exact commits are what make an installation
reproducible.

## Install the harness from the streaming fork

For an application managed by `uv`, install the harness branch directly:

```bash
uv add "temporal-agent-harness @ git+https://github.com/mfateev/temporal-agent-harness.git@task/external-workflow-streams"
```

For a reproducible application build, replace the branch name after the final `@` with the full
harness commit SHA.

The equivalent `pyproject.toml` configuration is:

```toml
[project]
dependencies = [
    "temporal-agent-harness[ui]",
]

[tool.uv.sources]
temporal-agent-harness = {
    git = "https://github.com/mfateev/temporal-agent-harness.git",
    branch = "task/external-workflow-streams",
}
```

Run `uv sync` after adding the source. Do not add a separate `temporalio` override to the consuming
application: the harness directly pins the streaming-capable Python SDK revision because `uv` does
not inherit a dependency package's source overrides.

## Run a checkout of the harness fork

Clone the streaming branch and install its locked Python and UI dependencies:

```bash
git clone --branch task/external-workflow-streams \
  https://github.com/mfateev/temporal-agent-harness.git
cd temporal-agent-harness
uv sync --frozen --group examples
cp .env.example .env.local
chmod 600 .env.local
just app-install
```

Set `OPENAI_API_KEY` and/or `GEMINI_API_KEY` in `.env.local`. The file is gitignored and must not
be committed. Redis 5 or newer must be reachable at the configured
`TEMPORAL_AGENT_HARNESS_REDIS_URL`; the default is `redis://127.0.0.1:6379/0`.

For the OpenAI example, start each process in a separate terminal:

```bash
just temporal
just session-manager
just server
just worker-openai-hello
```

The default harness UI is at <http://localhost:8000>. `just temporal` starts the local Temporal
service configured by `temporal.local.toml`.

## Check out and build the SDK stack

You do not need separate SDK checkouts merely to run the harness: `uv sync` installs the exact SDK
revision pinned by the harness. Use a source checkout when changing or validating the SDK itself.

Clone `sdk-python` with its recorded SDK Core revision:

```bash
git clone --branch task/python-sdk-streaming --recurse-submodules \
  https://github.com/mfateev/sdk-python.git
cd sdk-python
git submodule update --init --recursive
uv sync --all-extras
poe build-develop
```

The recorded submodule commit, not the current tip of the SDK Core branch, is the reproducible SDK
Core version for that `sdk-python` commit. A second standalone `sdk-core`/`sdk-rust` checkout is
optional and is not used by the Python SDK build.

Building this sibling SDK checkout does not automatically replace the Git-pinned SDK in the
harness virtual environment. To test an SDK change through the harness, first push the coordinated
SDK Core and Python SDK commits, then update the harness pin as described below.

## Advance the coordinated streaming branches

Keep the dependency chain ordered so every pushed harness commit identifies a buildable stack:

1. Commit and push SDK Core changes to `mfateev/sdk-core:task/python-sdk-streaming`.
2. In `sdk-python`, update `temporalio/bridge/sdk-core` to that exact commit, commit the submodule
   pointer, and push `mfateev/sdk-python:task/python-sdk-streaming`.
3. In the harness, replace the `temporalio` Git revision in `pyproject.toml` with the new full
   `sdk-python` commit SHA.
4. Run `uv lock` and `uv sync --frozen --group examples` so `uv.lock` and the environment use the
   same SDK revision.
5. Commit `pyproject.toml` and `uv.lock`, then push
   `mfateev/temporal-agent-harness:task/external-workflow-streams`.

Do not point the harness's `temporalio` dependency at a moving branch. An exact SDK commit prevents
the same harness revision from resolving to different SDK Core implementations over time.

## Verify the selected revisions

From the harness checkout, confirm the declared and locked Python SDK sources:

```bash
rg -n "mfateev/sdk-python|name = \"temporalio\"" pyproject.toml uv.lock
uv tree | rg temporalio
```

From the Python SDK checkout, confirm the branch and exact recorded SDK Core commit:

```bash
git status --short --branch
git submodule status temporalio/bridge/sdk-core
git -C temporalio/bridge/sdk-core remote -v
```

Before publishing a coordinated update, all three repositories should have clean worktrees and
zero commits ahead of or behind their intended upstream branches.
