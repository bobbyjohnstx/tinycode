---
name: release
description: Full tinycode release — version bump, changelog, docs, GitHub release, container build, operator update
params: [version]
---

# Release

End-to-end release workflow for the tinycode ecosystem. Covers version bump, changelog, documentation, GitHub release, container image, and operator update.

## When to use

- Cutting a new tinycode release (e.g., `/release v1.18.0`)
- The user says "release", "cut a release", "ship it", "new version"

## Prerequisites

Before starting, verify:

1. Working tree is clean (`git status` — no uncommitted changes)
2. All tests pass (`bun test` from `packages/tinycode/`)
3. Type check passes (`bun typecheck` from `packages/tinycode/`)
4. You're on the `main` branch

If any prerequisite fails, stop and report. Do not proceed with a dirty tree or failing tests.

## Phase 1: Determine Version

If `$1` was provided, use it (strip leading `v` for package.json, keep `v` prefix for tags).

If no version was provided, check the current version in `packages/tinycode/package.json` and the git log since the last release tag. Determine the appropriate bump:

- **patch** (x.y.Z) — only bug fixes
- **minor** (x.Y.0) — new features present
- **major** (X.0.0) — breaking changes

Confirm the version with the user before proceeding.

## Phase 2: Changelog

1. Run `git log --oneline` from the last release tag to HEAD
2. Categorize commits into Added, Changed, Fixed, Security sections
3. Update `CHANGELOG.md`:
   - Move items from `[Unreleased]` into a new `[version] — YYYY-MM-DD` section
   - Add all new commits not already in Unreleased
   - Keep the `[Unreleased]` header with empty sections for future work

## Phase 3: Version Bump

Bump version in ALL package.json files. They must stay in sync:

```bash
grep -rn '"version":' packages/*/package.json
```

Update every one to the new version. There are typically 8 packages:
- `packages/tinycode/package.json`
- `packages/app/package.json`
- `packages/desktop/package.json`
- `packages/plugin/package.json`
- `packages/llm/package.json`
- `packages/ui/package.json`
- `packages/http-recorder/package.json`
- `packages/effect-drizzle-sqlite/package.json`

Verify none were missed: `grep -rn '"version":' packages/*/package.json | grep -v $VERSION` should return nothing.

## Phase 4: Documentation Update

Review and update documentation for any changes since the last release:

1. **`packages/tinycode/src/cli/cmd/tui/help.md`** — keybindings, commands, features
2. **`docs/cheatsheet.md`** — keyboard shortcuts, agent list, workflow examples
3. **`docs/user-guide.md`** — keybinding tables, navigation, feature descriptions
4. **`CLAUDE.md`** — key patterns section for new architectural patterns
5. **`docs/UI-UX-REVIEW.md`** — mark resolved issues

Check for stale references:
```bash
# Find keybinding mismatches between code and docs
grep -n "keybind(" packages/tinycode/src/cli/cmd/tui/config/keybind.ts | head -30
# Compare against docs
grep -n "leader" docs/cheatsheet.md docs/user-guide.md packages/tinycode/src/cli/cmd/tui/help.md
```

## Phase 5: Commit and Tag

1. Stage all changed files (version bumps, changelog, docs)
2. Commit: `chore: bump version to $VERSION`
3. Push to both remotes:
   ```bash
   git push origin main
   git push github main
   ```
4. Create and push tag:
   ```bash
   git tag v$VERSION
   git push github v$VERSION
   ```

The tag push triggers the GitHub Actions release workflow which:
- Builds 12 platform binaries
- Creates a GitHub Release with auto-generated notes
- Publishes to npm (tinycode-ai + platform packages + tinycode-plugin + tinycode-sdk)
- Builds and pushes Docker image to GHCR
- Updates AUR and Homebrew tap

## Phase 6: Verify Release Workflow

After pushing the tag, check the release workflow:

```bash
gh run list --repo bobbyjohnstx/tinycode --limit 3
```

If it fails, check logs:
```bash
gh run view <run-id> --repo bobbyjohnstx/tinycode --log-failed | tail -30
```

Common failure points:
- **npm 404**: A platform binary package wasn't built. Check the build step.
- **Docker buildx**: Needs QEMU + buildx setup. Fixed in release.yml — if it fails, verify the setup steps are present.
- **npm auth**: Check NPM_TOKEN secret is configured.
- **Plugin SDK not published**: `tinycode-plugin` and `tinycode-sdk` must be published at the same version as `tinycode-ai`. Verify:
  ```bash
  npm view tinycode-plugin@$VERSION version
  npm view tinycode-sdk@$VERSION version
  ```
  If either is missing, the CI publish step may not cover them — publish manually with `npm publish` from the relevant package directory.

## Phase 7: Container Image

The tinycode-container project at `/Users/bjohns/projects/tinycode-container` builds a separate container image with tinycode + oh-my-tiny.

1. If the CI workflow needs updates, edit `.github/workflows/build-push.yaml`
2. Commit and push:
   ```bash
   git -C /Users/bjohns/projects/tinycode-container push origin main
   git -C /Users/bjohns/projects/tinycode-container push github main
   ```
3. Trigger the container build via GitHub Actions:
   - URL: https://github.com/bobbyjohnstx/tinycode-container/actions/workflows/build-push.yaml
   - Click "Run workflow"
   - Set `version` = `v$VERSION`
   - Set `tinycode_ref` = `v$VERSION`

   Or via CLI (requires PAT with `actions` scope):
   ```bash
   gh workflow run "Build and Push Container Image" \
     --repo bobbyjohnstx/tinycode-container \
     -f version=v$VERSION -f tinycode_ref=v$VERSION
   ```

4. Verify the build completes and images are tagged on GHCR and Quay.

## Phase 8: Operator Update

The tinycode-operator at `/Users/bjohns/projects/tinycode-operator` manages Kubernetes/OpenShift deployments. It has its OWN version cadence — do NOT match it to the tinycode version. Ask the user what operator version to use.

Update ALL version references — there are many:

| File | What to change |
|------|---------------|
| `Makefile` | `VERSION ?= X.Y.Z` |
| `helm-charts/tinycode/Chart.yaml` | `version:` and `appVersion:` |
| `helm-charts/tinycode/values.yaml` | `image:` tag to new tinycode-container version |
| `bundle/manifests/tinycode-operator.clusterserviceversion.yaml` | `metadata.name`, `containerImage`, `version`, alm-examples image |
| `bundle/manifests/tinycode.dev_tinycodeinstances.yaml` | CRD default image tag |
| `config/catalog/catalogsource.yaml` | catalog image tag |
| `config/crd/tinycode.dev_tinycodeinstances.yaml` | CRD default image tag |
| `config/samples/*.yaml` | all sample CR image references |
| `catalog/tinycode-operator/catalog.yaml` | all version and image references |

Verify no old versions remain:
```bash
grep -rn "OLD_OP_VERSION\|tinycode-container:latest" /Users/bjohns/projects/tinycode-operator/ --include="*.yaml" --include="Makefile" | grep -v .git/
```

Commit and push to both remotes.

## Phase 8.5: Plugin Template Update

The tinycode-plugin-template at `/Users/bjohns/projects/tinycode-plugin-template` must track the published `tinycode-plugin` version.

1. Update `package.json` devDependency `tinycode-plugin` to `^$VERSION`
2. Verify the template still builds: `bun install && bun run build`
3. Commit and push:
   ```bash
   git -C /Users/bjohns/projects/tinycode-plugin-template add package.json
   git -C /Users/bjohns/projects/tinycode-plugin-template commit -m "chore: bump tinycode-plugin to ^$VERSION"
   git -C /Users/bjohns/projects/tinycode-plugin-template push origin main
   git -C /Users/bjohns/projects/tinycode-plugin-template push github main
   ```

## Phase 9: Post-Release Checklist

- [ ] GitHub Release is published (not draft)
- [ ] npm packages are live: `npm view tinycode-ai@$VERSION`
- [ ] Plugin SDK published: `npm view tinycode-plugin@$VERSION` and `npm view tinycode-sdk@$VERSION`
- [ ] Plugin template updated to `^$VERSION`
- [ ] Container image tagged on GHCR: `ghcr.io/bobbyjohnstx/tinycode-container:v$VERSION`
- [ ] Operator updated and pushed
- [ ] Close any Gitea issues resolved in this release
- [ ] Update GitHub release notes if auto-generated notes are insufficient

## Rollback

If the release has critical issues:

1. **npm**: `npm unpublish tinycode-ai@$VERSION` (within 72 hours)
2. **GitHub**: Delete the release and tag via `gh release delete` + `git push :refs/tags/v$VERSION`
3. **Container**: Re-tag `:latest` to the previous known-good version
4. **Operator**: Revert the commit and push
