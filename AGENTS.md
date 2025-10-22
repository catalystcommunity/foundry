# Foundry - A CLI for managing individual pieces of a Catalyst Community tech stack and workflows within that stack

## Project Status

The system is actively being developed. Join the [Catalyst Community Discord](https://discord.gg/sfNb9xRjPn) to discuss and contribute.

## Best Practices

- We do not use .env files except for Docker because that is required, everything else is just a config file in yaml
- We write tests for every feature including the happy path and error paths
- We only mock third party APIs, everything we can run locally we'll spin up a container based dev/test environment
- We ensure all tests pass before we mark tasks complete
- We try to separate concerns. Things should not try to control too many other things.
- We use semantic versioning and conventional commits.

## Documentation Guidelines

- **Task tracking and checklists are fine** - We don't know ahead of time if work spans multiple sessions
- **Be cautious about creating user documentation** - When in doubt about writing guides, how-tos, or reference docs, **ask first**
- User-facing documentation goes in `docs/` when explicitly requested or clearly needed
- Design and architecture docs go in root (DESIGN.md, implementation-tasks.md, etc.)
- Don't create documentation "just in case" - wait until it's actually needed

