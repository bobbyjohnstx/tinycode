# Repository Migration: tinycode → tinycode

Guide for replacing the original TypeScript `tinycode` repo with the Go rewrite `tinycode`.

## Overview

- **Approach**: Archive old repo, rename Go repo, update import paths
- **Estimated time**: 30 minutes
- **Risk**: Low — both repos preserved, no history lost

## Prerequisites

- Admin access to Gitea for both repos
- Go toolchain installed
- No in-flight PRs or branches on either repo

## Steps

### 1. Tag the old repo as end-of-life

```bash
cd ~/projects/tinycode
git tag -a v1-final -m "Final TypeScript release — superseded by Go rewrite"
git push origin v1-final
```

### 2. Archive the old repo on Gitea

1. Go to `http://localhost:3000/bjohns/tinycode/settings`
2. Rename the repo to `tinycode-ts`
3. Or: check "Archive this repository" to make it read-only

### 3. Rename the Go repo

1. Go to `http://localhost:3000/bjohns/tinycode/settings`
2. Change repository name to `tinycode`
3. Gitea will set up redirects from the old URL automatically

### 4. Update the Go module path

The module path in `go.mod` determines all import paths. This is the biggest change.

```bash
cd ~/projects/tinycode  # still the local directory name

# Update go.mod module path
sed -i '' 's|github.com/bobbyjohnstx/tinycode|github.com/bobbyjohnstx/tinycode|g' go.mod

# Update all Go import paths
find . -name '*.go' -not -path './vendor/*' -not -path './.claude/*' -not -path './node_modules/*' \
  -exec sed -i '' 's|github.com/bobbyjohnstx/tinycode|github.com/bobbyjohnstx/tinycode|g' {} +

# Verify no old references remain
grep -r "tinycode" --include="*.go" . | grep -v ".claude/" | grep -v "node_modules/"
# Should return nothing (or only comments/docs)
```

### 5. Update non-Go references

```bash
# Makefile, scripts, configs
sed -i '' 's|tinycode|tinycode|g' \
  Makefile \
  install.sh \
  script/release.sh \
  .goreleaser.yml \
  CLAUDE.md

# Verify
grep -r "tinycode" \
  Makefile install.sh script/release.sh .goreleaser.yml CLAUDE.md
```

### 6. Update git remote

```bash
# Update the remote URL
git remote set-url tinycode http://localhost:3000/bjohns/tinycode.git

# Rename the remote for clarity
git remote rename tinycode origin
```

### 7. Rename local directory (optional)

```bash
cd ~/projects
mv tinycode tinycode
cd tinycode
```

### 8. Verify everything builds

```bash
go mod tidy
go vet ./...
go test ./... -count=1 -timeout 120s
make build
./dist/tinycode version
```

### 9. Commit and push

```bash
git add -A
git commit -m "chore: rename module from tinycode to tinycode

Updated go.mod module path, all Go import paths, Makefile, scripts,
and configuration files."
git push origin main
```

### 10. Cut a new release

```bash
./script/release.sh v2.1.0
```

### 11. Update the install script URL

The install script header comment references the repo URL:
```bash
# Old: curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh
# Still works — this is now the Go version
```

### 12. Add a redirect notice to the old repo

If you archived instead of renaming the old repo, add a notice to its README:

```markdown
# ⚠️ This repository has been superseded

The Go rewrite is now the primary version:
**[bjohns/tinycode](http://localhost:3000/bjohns/tinycode)**

This TypeScript version is archived and no longer maintained.
```

## Rollback

If something goes wrong:
1. Rename `tinycode` back to `tinycode` on Gitea
2. Rename `tinycode-ts` back to `tinycode` on Gitea
3. Revert the go.mod/import path commit: `git revert HEAD`

## Post-migration checklist

- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] `make build` produces working binary
- [ ] `./dist/tinycode version` shows correct version
- [ ] `./script/release.sh` uses correct repo name
- [ ] `install.sh` downloads from correct URL
- [ ] Old repo has archive notice or redirect
- [ ] CI/CD pipelines updated (if any)
- [ ] Container/operator repos updated to reference new name (#56, #57)
- [ ] Any documentation referencing `tinycode` updated
