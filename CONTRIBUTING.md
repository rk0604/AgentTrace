# Contributing to AgentTrace

## Scope

AgentTrace accepts a completed JSON trace and returns a JSON attribution result.
It does not orchestrate agents, store run history, manage users, or provide a
dashboard.

Keep generic graph and attribution behavior inside `attrib`. Keep domain logic
inside demo or integration packages. Domain fields belong inside the generic
`input` and `output` JSON payloads.

## Development workflow

1. Create a focused branch or worktree from `main`.
2. Keep each change limited to one behavior or contract.
3. Add tests that demonstrate the intended behavior and important failures.
4. Run the full verification commands.
5. Submit a pull request with contract impact and remaining risk documented.

Do not commit directly to `main`. Do not modify another agent owned worktree.

## Local setup

Install the Go version declared in `go.mod` and Python 3.10 or newer.

Install the incident pipeline dependency:

```powershell
python -m pip install -r .\examples\incident_agent_pipeline\requirements.txt
```

Live model execution has an additional dependency:

```powershell
python -m pip install -r .\examples\incident_agent_pipeline\requirements-live.txt
```

## Verification

Run these commands from the repository root:

```powershell
gofmt -w .\attrib .\checkerconfig .\cmd .\documentdemo .\incidentdemo .\integration .\recorder .\toypipeline
go test -race ./...
go vet ./...
python -m unittest discover -s .\examples\incident_agent_pipeline\tests -v
```

Run Python recorder tests from `sdk\python`:

```powershell
python -m unittest -v
```

The continuous integration workflow also checks formatting, module integrity,
coverage, generated replay traces, cross language trace compatibility, and the
attribution failure exit gate.

## Compatibility

Treat these as versioned public contracts:

1. Trace JSON
2. Attribution result JSON
3. Checker configuration JSON
4. CLI flags and exit codes
5. Python recorder behavior

Backward incompatible changes require a version increment, migration notes, and
tests for both rejection and supported compatibility behavior.

## Code documentation

Document function inputs, outputs, and errors using the conventions already
present in the repository. Comments should explain decisions and nonobvious
logic rather than restating syntax.
