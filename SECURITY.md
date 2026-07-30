# Security Policy

## Supported version

Security fixes are applied to the latest code on `main`. Earlier commits and
unreleased agent branches are not supported releases.

## Reporting a vulnerability

Use GitHub private vulnerability reporting for this repository. Do not open a
public issue containing exploit details, credentials, private traces, model
inputs, model outputs, or evaluation context.

Include:

1. A concise description of the issue
2. A minimal reproduction
3. The affected command, package, or contract
4. The expected security impact
5. Any suggested mitigation

Remove secrets and personal data from every submitted trace. A response should
be expected within seven days.

## Sensitive data

Attribution output omits raw input, output, and expected context by default.
The `--include-evidence` flag can expose those payloads and should only be used
in an approved environment. Trace producers remain responsible for redacting
data before it enters a trace.
