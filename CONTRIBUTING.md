# Contributing

The **Senpan App Suite** is proprietary software (see [`LICENSE`](LICENSE)) - contributions are by invitation. By submitting a change you confirm you have the right to do so and agree it becomes part of the Software under that license.

Read [`AGENTS.md`](AGENTS.md) before any change: it is the deep guide to this repo, for developers and coding agents alike, with the setup, commands and conventions, and it links the shared standards every project follows. New to the repo? Start with [`README.md`](README.md).

## Reporting a bug

Open an issue that gives the steps to reproduce it, what you expected, what happened instead, and your environment: the browser or game client, the device, and the versions shown in the admin sidebar footer.

## Proposing a change

Open an issue first for anything beyond a small, obvious fix, so the change is agreed before the work starts.

## Submitting a change

- Work lands on `main`, with no feature branches.
- CI on GitHub is the gate: run `task check` (or `scripts/check.ps1`) before you push, and the change is done when CI passes on `main`. No git hook runs the checks.
- Commits, versions and the changelog follow the shared rules in [`shared/standards/docs-and-agents.md`](shared/standards/docs-and-agents.md); AGENTS.md says which version and which changelog section a change bumps.
