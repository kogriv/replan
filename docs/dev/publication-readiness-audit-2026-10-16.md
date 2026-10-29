# Publication readiness audit 2026-10-16

Status: agent audit; the repository stays private, the operator decides the
open items before it is made public.

Task: `task.self.license-and-audit-repo-for-publication`.

Follows the [OSS boundary checkpoint of 2026-08-02](oss-boundary-checkpoint-2026-08-02.md)
and rechecks its five blockers against today's tree and history.

## Method

- Every tracked file, 472 at the start, scanned by category: keys, tokens and
  passwords; host names, IP addresses and server paths; people and accounts;
  the operator's other projects; real OpenProject work-package ids; Cyrillic
  text in product files; file types, sizes and generated files.
- The whole history, 356 commits on all branches, scanned with
  `git log --all -G` for the same private patterns, including lines that were
  removed later.
- Licences: the root of the repository, the two OpenProject plugin gemspecs,
  the Go dependencies.

The private patterns themselves, the operator's host names, paths and
account, are not written in this file. They are kept in the ignored file
`private/leak-patterns.txt`, which the agent kit check reads.

## Fixed in this task

- License: `LICENSE` (Apache-2.0, the canonical text), `NOTICE`, a README
  section, and both plugin gemspecs, which said MIT, now say Apache-2.0.
  OpenProject is GPLv3; Apache-2.0 code may be combined with it.
- The agent kit check had the operator's private host, path and account
  names written in its leak patterns. Those patterns now come from the
  ignored private file; the script keeps only generic ones. Checked three
  ways: with the private file, without it, and with a probe pattern that must
  fail.
- The 2026-08-02 checkpoint printed the same private names in its command
  list; replaced with placeholders.
- The identifier of one of the operator's other projects appeared as a
  sample project id in four design documents; now `demo`.
- Three real work-package ids from the pilot appeared in a JSON example; now
  sample ids.
- Samples that named the owner's other GitHub repositories now use
  `example/...`.
- The local checkout path in the README and in two CLI output samples is
  generic.

## Not found

- No live token, API key, perimeter key or private key, in the tree or in the
  history. The only passwords are those of throwaway test users created by the
  plugin checks.
- No tracked private folder, `.env` file, generated report or build output.
- No name, e-mail address or account of a person other than the repository
  owner's public GitHub account.
- No vendored third-party code. The one Go dependency, `gopkg.in/yaml.v3`, is
  MIT and Apache-2.0.

## Open before publication

Each needs the operator's decision; the recommendation is the agent's.

1. **History (blocker 5 of 2026-08-02, still open).** The history holds what
   the tree no longer does. Two commits of 18 July carried the OpenProject
   server's public host name and IP address, removed two days later. About
   twenty commits mention the private connection port, and some commits the
   local checkout path. Publishing this history publishes them.
   *Recommendation:* publish a new repository, or a new orphan branch, from
   the sanitized tree with one first commit, and keep this history private.
   Rewriting the history is the alternative, but the old commits stay on the
   existing GitHub and GitLab remotes.
   **Resolved 2026-10-28:** the operator chose a new public repository with one
   first commit from the sanitized `HEAD`; `scripts/cut-public-tree.sh` prepares
   and verifies that repository, and the current history stays private. Creating,
   renaming and pushing repositories remains the operator's step under the
   [public repository cut runbook](public-repository-cut.md).
2. **Pilot narrative (blocker 3, still open).**
   `docs/spec/openproject-pilot-log.md` (895 lines; moved to private storage on
   2026-10-27), `docs/spec/openproject-live-mapping-quality.md` (2 081 lines;
   moved to private storage on 2026-10-27) and the
   historical sections of `docs/dev/getting-started.md` read as the operations
   journal of a private OpenProject installation: hosting and connection
   setup, pilot users and ids.
   *Recommendation:* move them to private storage and keep a short sanitized
   page of what they taught.
   **Resolved 2026-10-27:** the named files and old getting-started history
   were moved to private storage; `openproject-pilot-lessons.md` keeps the
   sanitized product lessons.
3. **Pilot sidecars (blocker 4, still open).**
   `examples/openproject/sidecars/*.replan-pilot.yaml` (moved to private
   storage on 2026-10-27) carried old pilot ids.
   *Recommendation:* delete them; the `replan-self` sidecars show the same
   format.
   **Resolved 2026-10-27:** the two pilot sidecars were deleted from the public
   tree; the `replan-self` examples remain.
4. **`CONTRIBUTING.md` and `SECURITY.md` (blocker 2, still open).** A security
   policy needs a contact address for reports.
   *Recommendation:* add both when the publication date is set, with an
   address chosen for it.
   **Resolved 2026-10-28:** both root entry points are present. `SECURITY.md`
   directs reports only to GitHub private vulnerability reporting and does not
   publish an e-mail address. On publication, enable private vulnerability
   reporting in the new GitHub repository's settings; the documented report
   path depends on it.
5. **The self-dogfood project.** `examples/replan-self` and the roadmap's
   calibration steps record three months of real work: dates, hours, the
   operator's decisions, and mentions of a second and a third project by role.
   The boundary allows it, as the ids are sanitized, and it is the strongest
   demonstration of the product.
   *Recommendation:* publish it as it is. Reread the roadmap's last steps for
   mentions of the other projects before the publication date.
   **Resolved 2026-10-28:** the roadmap from 2026-10-15 onward and every
   `examples/replan-self` task description were reread. They name the other
   work only by role, such as a second or third project, an outside
   administrator, a dispatcher or a peer session. One task description named
   the operator's code-analysis project; it now says "another code-analysis
   project." No other-project name or identifier remains in that reviewed
   scope. Publishing remains the operator's step under the
   [runbook](public-repository-cut.md).
6. **The owner's GitHub account** is the Go module path
   (`github.com/<owner>/replan`), the gemspec homepage, and an actor id in two
   samples. That is public by nature if the repository is published under
   that account.
   *Recommendation:* keep it, or move the module path if the repository is
   published under an organization.
   **Resolved 2026-10-28:** the public repository stays under the owner's
   account, so `go.mod`, Go imports, plugin repository URLs and public sample
   account references keep the existing owner. The cut helper reports these
   module-owner exemptions while scanning every other private pattern.
   Publishing remains the operator's step under the
   [runbook](public-repository-cut.md).
7. **Russian text.** Five lines in two documents quote the operator's words
   in Russian.
   *Recommendation:* keep them as quotes with the English rendering beside
   them.
   **Resolved 2026-10-28:** all five Russian source lines remain quoted and now
   have an adjacent English rendering. Publishing remains the operator's step
   under the [runbook](public-repository-cut.md).
8. **The agent kit's terms of use** say the kit is not open-source. While the
   repository is private that is consistent: the owner may hand out builds
   under other terms.
   *Recommendation:* switch the kit's `NOTICE.md` to Apache-2.0 on the day the
   repository is published.
   **Resolved 2026-10-28:** the kit now carries the repository's Apache-2.0
   `LICENSE` and `NOTICE`; its `Provided for:` line is an informational purpose
   note, not an additional term. Publishing remains the operator's step under
   the [runbook](public-repository-cut.md).
9. **Dated records** name the local checkout path: the audits of
   2026-09-21 and 2026-09-25, the research note of 2026-07-02 and the initial
   roadmap. Three gap records and plans name the private connection port they
   genericized.
   *Recommendation:* keep them as dated records. If the history is published
   anyway (item 1), these details are there regardless.
   **Resolved 2026-10-28:** the dated records remain as dated records, while the
   chosen one-commit public repository excludes the private history that held
   removed values. The cut helper repeats generic and private-pattern checks on
   the exact public tree. Publishing remains the operator's step under the
   [runbook](public-repository-cut.md).
