# CLAUDE.md

Supplemental guidance for Anthropic Claude when working in this repository. Read `AGENTS.md` first for shared conventions, then apply the adjustments below.

## Collaboration Style

- Think out loud when the task is ambiguous, but keep the final answer concise
- When tackling multi-step work, outline the intended plan before running commands and update it as you progress
- Prefer actionable bullet points over prose when surfacing findings or next steps

## Tooling Notes

- Use the provided CLI tools and scripts referenced in `AGENTS.md`; avoid launching alternative package managers or editors
- Treat any long-running or potentially destructive command as opt-in—confirm with the user before executing

## Sandbox Awareness

- The Claude harness may queue shell commands; group related operations where possible to reduce round-trips
- Surface permission or sandbox limitations immediately and suggest workarounds rather than retrying blindly

## When In Doubt

- Link back to the relevant section in `AGENTS.md` instead of duplicating content
- Ask the user for clarification whenever expectations conflict or the repository appears out of sync with the documented workflows
- When working with our catnip-dev docker container, dont restart or mess with the catnip process. Air is configured to rebuild and restart when changes are made. If you can build the app locally, air has already done it in the container and we can proceed.
- When checking if swift changes build successfully use `just build` or `just build-quick` from within the xcode directory.
