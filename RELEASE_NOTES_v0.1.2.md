# Clash for Claw v0.1.2

## Highlights

- Refined packaging into `full` and `slim`, with `full` as the default GitHub-ready release profile.
- Validated the adapter against a live OpenClaw environment without modifying the local OpenClaw repository or system proxy.
- Tightened release hygiene by removing local diagnostics, temporary outputs, and one-off test artifacts from the tracked workspace.
- Kept the desktop app and backend sidecar decoupled so OpenClaw repo updates remain independent.

## Files

- `Clash-for-Claw-0.1.2-full-setup.exe`: recommended installer for most users
- `Clash-for-Claw-0.1.2-full-portable-win-x64.zip`: portable package with bundled `mihomo`
- `Clash-for-Claw-0.1.2-slim-setup.exe`: smaller installer that keeps runtime download behavior
- `Clash-for-Claw-0.1.2-slim-portable-win-x64.zip`: smallest portable package
