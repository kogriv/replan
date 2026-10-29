# Public repository cut

Date: 2026-10-28

Status: operator runbook. Publishing and repository administration are always
the operator's steps.

## 1. Inspect the current checkout and choose private placeholders

Start in the existing RePlan checkout. Do not paste real repository URLs into
tracked files or command logs intended for publication.

```sh
git status --short
git remote -v
export REPLAN_PUBLIC_CUT_DIR="$HOME/<PUBLIC-CUT-DIRECTORY>"
export REPLAN_PUBLIC_CUT_AUTHOR_NAME="<PUBLIC-COMMIT-AUTHOR>"
export REPLAN_PUBLIC_CUT_AUTHOR_EMAIL="<PUBLIC-COMMIT-EMAIL>"
export REPLAN_PUBLIC_CUT_DATE="<RFC3339-PUBLICATION-DATE>" # optional
```

Verify that `git status --short` is empty, the intended GitHub and GitLab
destinations are understood, and `REPLAN_PUBLIC_CUT_DIR` is outside this
checkout and either absent or an empty directory.

## 2. Run and review the public cut

Dry run first:

```sh
scripts/cut-public-tree.sh > "$HOME/<PUBLIC-CUT-DRY-RUN-REPORT>.json"
python3 -m json.tool "$HOME/<PUBLIC-CUT-DRY-RUN-REPORT>.json"
```

Review every named check, warning, module-owner exemption, the file count,
total size and `source_tree_match`. The dry run must report `ok: true`,
`mode: dry_run`, and equal `tree_sha` and `source_tree_sha`.

Apply only after that review:

```sh
REPLAN_APPLY=1 scripts/cut-public-tree.sh > "$HOME/<PUBLIC-CUT-APPLY-REPORT>.json"
python3 -m json.tool "$HOME/<PUBLIC-CUT-APPLY-REPORT>.json"
git -C "$REPLAN_PUBLIC_CUT_DIR" log --oneline --decorate --all
git -C "$REPLAN_PUBLIC_CUT_DIR" remote -v
```

Verify one commit on `main`, the reported commit and tree SHAs, and no remotes.

## 3. Archive the current GitHub and GitLab repositories

In GitHub, rename the current repository from the public name to a private
history name such as `replan-history`, confirm that it remains private, and
disable ordinary development there. Create a new public repository under the
old `replan` name without a generated README, licence or initial commit.

Do the equivalent for the GitLab repository that `git remote -v` shows as the
current push mirror: rename the existing repository to a private history name,
keep it private, and create a new public empty repository under the old name.
Do not reuse a displayed private URL in documentation; use these placeholders:

```text
<NEW-GITHUB-REPLAN-URL>
<NEW-GITLAB-REPLAN-URL>
<PRIVATE-GITHUB-HISTORY-URL>
<PRIVATE-GITLAB-HISTORY-URL>
```

GitHub redirects the old name to a renamed repository until a new repository
takes that name; create the new public repository right after the rename, so no
push or clone reaches the history repository by mistake.

Verify in both hosting interfaces that the history repositories are private,
the new repositories are empty and public, and their names are correct.

## 4. Push the cut and enable public-repository settings

From the cut repository, push the same `main` commit to both new empty
repositories:

```sh
cd "$REPLAN_PUBLIC_CUT_DIR"
git remote add origin <NEW-GITHUB-REPLAN-URL>
git remote add gitlab <NEW-GITLAB-REPLAN-URL>
git push -u origin main
git push gitlab main
git remote -v
```

Verify that both hosts show the one public commit and the tree SHA from the
apply report. In GitHub, enable private vulnerability reporting because
`SECURITY.md` depends on it. Enable the GitHub Actions workflow if public CI is
wanted; review its first run rather than assuming that publication enabled it.

## 5. Preserve the old history in the existing checkout

Return to the original checkout. Keep its current history reachable before
changing `main` or `origin`:

```sh
cd <EXISTING-REPLAN-CHECKOUT>
git status --short
git branch history-main main
git remote rename origin history
git remote -v
```

Verify a clean status, `history-main` at the old tip, and the old private
GitHub/GitLab destinations still reachable through the `history` remote.

## 6. Fetch and prove that the public tree is identical

Add the new GitHub repository temporarily as `public`, fetch it, and compare
trees before changing the checked-out branch:

```sh
git remote add public <NEW-GITHUB-REPLAN-URL>
git fetch public main
test "$(git rev-parse HEAD^{tree})" = "$(git rev-parse public/main^{tree})"
git status --short
test -d private
test -f .env.local
```

The tree comparison and both local-file checks must succeed, and status must
remain clean. If an installation intentionally lacks either ignored path,
verify its expected ignored files explicitly with `git check-ignore -v`
instead of creating a placeholder.

## 7. Point `main` and `origin` at the public history in place

Because the trees match, replace only the branch history; do not move or
re-clone the directory:

```sh
git checkout -B main public/main
git remote rename public origin
git branch --set-upstream-to=origin/main main
git remote set-url --add --push origin <NEW-GITHUB-REPLAN-URL>
git remote set-url --add --push origin <NEW-GITLAB-REPLAN-URL>
git status --short
git remote -v
git log --oneline --decorate --all -n 5
test -d private
test -f .env.local
```

Verify that `main` has the one-commit public history, `origin/main` is its
upstream, both new repositories are push destinations, `history-main` and the
`history` remote preserve the private history, status is clean, and the ignored
`private/` and `.env.local` data are still present at the same path.

## 8. Run the post-switch local gate

```sh
go test ./...
git status --short
```

Verify that tests pass, status is clean, ignored local files remain ignored,
and the public hosts still show the same tree. Development continues in the
new public repository. Treat the private history repositories and
`history-main` as a read-only archive, not as a second development line.
