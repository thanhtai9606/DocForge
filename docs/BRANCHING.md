# Git branching model

## Branches

| Branch | Role |
|--------|------|
| `develop` | Default integration branch. All feature/fix PRs merge here first. |
| `main` | Release branch. Receives promotions from `develop`. GitHub Actions image publish runs here. |

## Flow

```text
feature/* ──PR──▶ develop ──PR──▶ main ──▶ GHCR publish
```

1. Create feature branches from `develop`.
2. Open PRs targeting **`develop`**.
3. When ready to release, open a PR `develop` → `main`.
4. Push/`merge` to `main` (or `v*` tags) triggers image build/publish to GHCR.

## CI policy

`.github/workflows/docker-publish.yml` runs only on:

- `push` to `main`
- version tags `v*`
- `workflow_dispatch`

It does **not** run on `develop` pushes or pull requests.
