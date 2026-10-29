# Code quality tooling evaluation 2026-09-24

Status: bounded trial completed; recommendation awaits operator decision.

Date: 2026-09-24.

Related task: `task.self.evaluate-code-graph-quality-tooling`.

## Question

Is it worth using graph- and semantics-based code analysis tools on the RePlan
codebase to find inconsistency, duplication, dead code and concentration of
behavior? Which ones, and in what role?

Candidate source: the tool survey of another code-analysis project
(`research/`), which measured seven code-graph and semantic tools hands-on on
one Python target. That project itself analyzes Python only.

## What RePlan Is, For A Tool

```text
Go, non-test:              18 376 lines, 5 packages
largest file:              cmd/replan/main.go, 4 723 lines
shell scripts:             24 files, 7 373 lines
Python inside shell:       22 heredoc blocks, 3 916 lines
Python and Ruby files:     443 and 758 lines
```

Two facts decide most of the answer:

1. Go forbids import cycles between packages, and RePlan has five packages.
   Whole-graph questions about cycles and layering, that project's main
   strength, are either answered by the compiler or trivial here. The only
   notable
   layering fact is that `internal/storage/filestore` imports `internal/app`,
   because the store implements the application's writer interfaces.
2. More than half of the script logic is Python written inside shell
   heredocs. No Python tool, no Go tool and no graph index sees it as code.
   The duplicated OpenProject auth header construction that the 2026-09-21
   audit found lives exactly there.

## Trial

All tools ran on a `git archive HEAD` copy in a scratch directory. Nothing in
the repository was changed by a tool.

### Go-native analyzers

| Tool | Result on RePlan | Real signal |
| --- | --- | --- |
| `staticcheck` 2025.1.1 | 2 findings | one unused function, one simplification |
| `deadcode` v0.31.0 | 5 unreachable functions | the same unused function; four `*DryRun` service wrappers that only tests call, while the CLI passes the dry-run flag in the request |
| `gocyclo` / `gocognit` | 14 functions over cyclomatic 30 | `validateWorkspace` 93 / cognitive 148, `WriteProjectStateMarkdown` 79 / 143, `ImportAcceptService.Accept` 63 / 124 |
| `dupl`, 150 tokens | 10 clone groups | see below |
| `jscpd` 4, 70 tokens, all languages | 73 clones, 1 339 duplicated lines, 6.95% of Go | 685 duplicated lines inside `main.go`, 242 inside `actual.go` |
| `shellcheck` | 29 findings in 24 scripts | 23 are the intentional `CDPATH= cd` idiom; about six worth fixing |

Duplication that matters, because it touches code the backlog is about to
change:

- `internal/storage/filestore/store.go` repeats the same read, unmarshal,
  find, mutate, marshal and atomic-write sequence in each writer method. The
  atomicity fix of `task.self.fix-actual-audit-ids-and-demand-reopen` would
  have to be made in every copy. One generic update helper first makes that
  fix a single change;
- `ImportAcceptService.Accept` has four near-identical 50-line branches, one
  per accept mode;
- three OpenProject page fetchers in `import_openproject.go` differ only in
  the collection type;
- `main.go` carries most of its duplication in per-command Markdown writers
  and flag parsing.

### Graph tool: GitNexus 1.6.9

Indexed in 20 seconds, 1.2 GB peak memory: 5 083 nodes, 14 933 edges, 86
clusters, 293 flows. Checked against ground truth from `grep`:

- callers of `Store.SetResourceDemandRemaining`: correct production caller,
  resolved through the `ActualWriter` interface, answer marked
  `lower-bound`. The test caller was not listed;
- the execution flows through that method start at the dead `*DryRun`
  wrappers, not at the CLI entry point. A flow view seeded from dead code
  reads as a real path;
- `check --cycles`: clean, which in Go is guaranteed by the compiler;
- concept query "OpenProject basic auth header": found three of the five
  shell scripts that build the header, as files. It cannot point at the code
  inside them, because heredoc Python is not parsed.

### Not run

CodeGraph, OntoIndex, graphlens, cocoindex-code and Sentrux. The other
project's cards already measured the first four on a larger target; their
failure modes are
the same class as GitNexus's here: point questions right, whole-graph answers
absent or noisy, non-diffable artifacts. Sentrux is unmeasured anywhere.

## Lessons Taken From Another Code-Analysis Project

The useful transfer is methodological, not a tool:

- **A partial map produces confident wrong answers.** `jscpd` with default
  settings skips files over 100 KB and silently reported 2.43% duplication on
  5 095 Go lines. It had left out `main.go`. With the limit lifted the real
  figure is 6.95% on 18 376 lines. The embedded Python is the same problem at
  a larger scale: every tool above reports on a map that has a 3 916-line
  hole, and none says so;
- **the tool cannot judge its own recall.** Every number above was checked
  against `grep` or the source before it went into this document;
- **whole-graph questions need a whole graph.** For RePlan that means moving
  the heredoc Python into real modules before any cross-language analysis is
  trusted.

## Recommendation

1. **Adopt the Go-native set as a report, then as a gate on new code.**
   `staticcheck`, `deadcode`, a complexity report and `dupl`, preferably
   through one `golangci-lint` configuration pinned to a version. First run it
   as a baseline report; then gate only new findings in CI. Cost is minutes;
   licenses are permissive; output is deterministic and diffable.
2. **Add `shellcheck` to CI** with the `CDPATH` idiom excluded.
3. **Move the heredoc Python into `scripts/lib/` modules.** This is the
   largest structural finding. It removes the auth-header duplication, makes
   the helpers testable and makes them visible to every analyzer.
4. **Extract the store update helper before the actual-audit fix.** Fold it
   into that task, so the atomicity change is made once.
5. **Use graph tools ad hoc, not as a gate.** GitNexus is already configured
   as an agent tool on this machine and is useful for impact questions before
   a refactor of `main.go`. Its license is non-commercial, its index is a
   binary store and its flows can start from dead code, so it does not belong
   in CI or in public project tooling.
6. **Do not adopt that project's tool for RePlan.** It is Python-only by
   design, and the questions it answers best are the ones Go already answers
   for RePlan.

Items 1 to 4 become backlog work only after the operator accepts them.
