# File format compatibility

RePlan format 1 covers the complete project on disk, not only
`.replan/project.yaml`. Releases that continue to write format 1 may make only
additive changes:

- add optional fields that are omitted when empty;
- add optional files or directories;
- add enum values when validation of every older value stays unchanged;
- add diagnostics.

A change needs a new format version and an ordered `replan project migrate`
step when it renames or removes a field, changes a field's type or meaning,
makes an optional field required, moves files, or changes how ids are formed.
Migration never downgrades a project.

The loader uses `yaml.v3` without `KnownFields`, so it ignores fields it does
not know. The canonical YAML splicer preserves unknown mapping fields during
the supported record and field writes. If a document cannot be spliced and a
writer falls back to rendering the whole typed document, unmodeled fields can
be lost; callers should not use an older binary to rewrite data that depends
on newer fields.

A binary that sees a format version newer than it supports stops after reading
`.replan/project.yaml`. It reports `project_format_unsupported` and does not
load the other project files.

Sealed baselines and calibrations are SHA-256 digests of their compact JSON
encoding with the seal itself omitted. Any new field in a sealed type must use
`omitempty` in both its JSON and YAML tags so its zero value does not change
old digests or appear in old files. Otherwise every existing seal breaks.

JSON command envelopes and other golden command outputs are API/test contracts;
they are not part of the project file format.

Frozen projects from tagged releases live in
`internal/app/testdata/format-v1`. The app-layer compatibility test loads,
validates, reports, calculates workload, checks migration, and verifies seals
for each copy. Fixtures are added from tags and never edited.
