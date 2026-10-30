#!/usr/bin/env bash
# Build the RePlan agent kit: an archive an outside person or agent can work
# from without this repository (docs/agent-kit/README.md). It carries the
# replan binary, the OpenProject helpers, a sample plan, the settings template
# and the guide; no Go sources, no plugin sources, no private files.
#
# Settings:
#   REPLAN_AGENT_KIT_DIR      default dist
#   REPLAN_AGENT_KIT_VERSION  default current git date and revision
#   REPLAN_AGENT_KIT_PURPOSE  default work agreed with the owner
#   TMPDIR                    default /tmp
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/build-agent-kit.sh            # prints the archive's path
#   REPLAN_AGENT_KIT_PURPOSE="work on project X" scripts/build-agent-kit.sh
#   REPLAN_AGENT_KIT_DIR=dist scripts/build-agent-kit.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "build-agent-kit.sh" "$@"
OUT_DIR="${REPLAN_AGENT_KIT_DIR:-$ROOT_DIR/dist}"
VERSION="${REPLAN_AGENT_KIT_VERSION:-$(cd "$ROOT_DIR" && git log -1 --format=%cd-%h --date=format:%Y%m%d 2>/dev/null || echo dev)}"
COMMIT="$(cd "$ROOT_DIR" && git rev-parse HEAD 2>/dev/null || echo dev)"
BUILD_DATE="$(cd "$ROOT_DIR" && git show -s --format=%cI HEAD 2>/dev/null || echo dev)"
NAME="replan-agent-kit-$VERSION-linux-amd64"

# The helpers an outside agent needs; plugin builds and runtime checks stay
# in the repository.
SCRIPTS="
accept-task.sh
accept-openproject-soft-drift.sh
apply-openproject-bootstrap.sh
apply-openproject-relations.sh
build-openproject-live-overlay.sh
check-openproject-drift.sh
check-openproject-live-alignment.sh
check-resource-schedule-drift.sh
explain-resource-overload.sh
openproject-operator-report.sh
openproject-wiki.sh
openproject-work-package-toolkit.sh
plan-openproject-bootstrap.sh
plan-resource-demand-calibration.sh
publish-openproject-overlay.sh
push-openproject-date-drift.sh
push-openproject-status-drift.sh
review-resource-pool.sh
validate-openproject-live.sh
"
LIBS="
accept_task.py
accept_openproject_soft_drift.py
apply_openproject_bootstrap.py
apply_openproject_relations.py
check_openproject_drift.py
check_openproject_live_alignment.py
check_resource_schedule_drift.py
envelope.sh
envelope_check.py
explain_resource_overload.py
helper_envelope.py
import_candidates.py
load-dotenv.sh
settings.list
openproject_http.py
openproject_bindings.py
openproject_import_failure.py
openproject_live_overlay.py
openproject_operator_report.py
openproject_rescheduling.py
openproject_status_map.json
openproject_status_map.py
openproject_working_days.py
openproject_wiki.py
openproject_work_package_toolkit.py
plan_openproject_bootstrap.py
plan_resource_demand_calibration.py
push_openproject_date_drift.py
push_openproject_status_drift.py
replan_workload.py
review_resource_pool.py
"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-agent-kit.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM
KIT="$WORK_DIR/$NAME"
mkdir -p "$KIT/bin" "$KIT/scripts/lib" "$KIT/examples"

LDFLAGS="-s -w -X github.com/kogriv/replan/internal/buildinfo.Version=$VERSION -X github.com/kogriv/replan/internal/buildinfo.Commit=$COMMIT -X github.com/kogriv/replan/internal/buildinfo.BuildDate=$BUILD_DATE"
# The commit and date come from ldflags; VCS stamping would fail under a stray
# .git above TMPDIR and mark a clean worktree +dirty.
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "$LDFLAGS" -o "$KIT/bin/replan" ./cmd/replan)

for script in $SCRIPTS; do
  install -m 755 "$ROOT_DIR/scripts/$script" "$KIT/scripts/$script"
done
for lib in $LIBS; do
  install -m 644 "$ROOT_DIR/scripts/lib/$lib" "$KIT/scripts/lib/$lib"
done
cp -R "$ROOT_DIR/examples/small-team" "$KIT/examples/small-team"
rm -rf "$KIT/examples/small-team/reports/latest"
install -m 644 "$ROOT_DIR/docs/agent-kit/README.md" "$KIT/README.md"
install -m 644 "$ROOT_DIR/docs/agent-kit/env.local.example" "$KIT/env.local.example"
# The repository licence and notice travel with the kit. What the archive is
# handed over for is an informational note set at build time, because it may
# name a private project.
install -m 644 "$ROOT_DIR/LICENSE" "$KIT/LICENSE"
PURPOSE="${REPLAN_AGENT_KIT_PURPOSE:-work agreed with the owner}"
sed "s|@PURPOSE@|$PURPOSE|" "$ROOT_DIR/docs/agent-kit/NOTICE.md" > "$KIT/NOTICE.md"
chmod 644 "$KIT/NOTICE.md"
printf '%s\n' "$VERSION" > "$KIT/VERSION"

mkdir -p "$OUT_DIR"
tar -C "$WORK_DIR" --owner=0 --group=0 --numeric-owner -czf "$OUT_DIR/$NAME.tar.gz" "$NAME"
(cd "$OUT_DIR" && sha256sum "$NAME.tar.gz" > "$NAME.tar.gz.sha256")
printf '%s\n' "$OUT_DIR/$NAME.tar.gz"
