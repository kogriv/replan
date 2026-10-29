package reports

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const reportHTMLStyle = `<style>
:root,
:root[data-theme="light"] {
  color-scheme: light;
  --bg: #f7f7f4;
  --fg: #1f2522;
  --muted: #626b65;
  --line: #d9ded8;
  --panel: #ffffff;
  --warning: #9a5b00;
  --danger: #b3261e;
  --ok: #23704a;
}
:root[data-theme="dark"] {
  color-scheme: dark;
  --bg: #111411;
  --fg: #e5e8e3;
  --muted: #a1aa9f;
  --line: #343a34;
  --panel: #1a1f1b;
  --warning: #f0b35a;
  --danger: #ff8a80;
  --ok: #7bd49d;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    color-scheme: dark;
    --bg: #111411;
    --fg: #e5e8e3;
    --muted: #a1aa9f;
    --line: #343a34;
    --panel: #1a1f1b;
    --warning: #f0b35a;
    --danger: #ff8a80;
    --ok: #7bd49d;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0;
  overflow-x: hidden;
  background: var(--bg);
  color: var(--fg);
  font: 14px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
main {
  max-width: 1180px;
  margin: 0 auto;
  padding: 28px 20px 48px;
}
header { margin-bottom: 24px; }
.topbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
h1, h2 { margin: 0; line-height: 1.15; }
h1 { font-size: 30px; font-weight: 700; }
h2 { margin-top: 30px; margin-bottom: 12px; font-size: 18px; }
.meta { margin-top: 10px; color: var(--muted); }
.summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 10px;
  margin: 18px 0 6px;
}
.metric {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px;
}
.metric span { display: block; color: var(--muted); font-size: 12px; }
.metric strong { display: block; margin-top: 4px; font-size: 24px; }
table {
  width: 100%;
  border-collapse: collapse;
  background: var(--panel);
  border: 1px solid var(--line);
}
caption { padding: 8px 10px; text-align: left; white-space: normal; }
.table-scroll { max-width: 100%; overflow-x: auto; }
.table-scroll table { min-width: max-content; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
th, td {
  padding: 9px 10px;
  border-bottom: 1px solid var(--line);
  text-align: left;
  vertical-align: top;
}
th { color: var(--muted); font-size: 12px; font-weight: 600; }
td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
tr.overloaded td { background: #fff6e5; }
.load-zero { background: #eef1ee; }
.load-low { background: #cce8d6; }
.load-medium { background: #f8d58d; }
.load-over { background: #f5b8b2; }
.heat-legend { display: flex; flex-wrap: wrap; gap: 12px; margin: 8px 0 12px; }
.heat-legend span { display: inline-flex; align-items: center; gap: 5px; }
.heat-swatch { width: 16px; height: 16px; border: 1px solid var(--line); }
details { margin-top: 6px; }
summary { cursor: pointer; }
pre.command { overflow-x: auto; padding: 9px 10px; border: 1px solid var(--line); background: var(--bg); white-space: pre-wrap; }
:root[data-theme="dark"] tr.overloaded td { background: #342717; }
:root[data-theme="dark"] .load-zero { background: #242924; }
:root[data-theme="dark"] .load-low { background: #28513a; }
:root[data-theme="dark"] .load-medium { background: #684d1d; }
:root[data-theme="dark"] .load-over { background: #6a302b; }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) tr.overloaded td { background: #342717; }
  :root:not([data-theme="light"]) .load-zero { background: #242924; }
  :root:not([data-theme="light"]) .load-low { background: #28513a; }
  :root:not([data-theme="light"]) .load-medium { background: #684d1d; }
  :root:not([data-theme="light"]) .load-over { background: #6a302b; }
}
.theme-switcher {
  display: inline-flex;
  gap: 4px;
  padding: 3px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
}
.theme-switcher button {
  border: 0;
  border-radius: 4px;
  padding: 6px 9px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
}
.theme-switcher button[aria-pressed="true"] {
  background: var(--fg);
  color: var(--bg);
}
.code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
.muted { color: var(--muted); }
.warning { color: var(--warning); }
.error { color: var(--danger); }
.ok { color: var(--ok); }
.view-links a { margin-right: 10px; }
.badge {
  display: inline-block;
  margin: 3px 4px 0 0;
  padding: 2px 6px;
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--muted);
  font-size: 11px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: top;
  white-space: nowrap;
}
.badge.warning { color: var(--warning); border-color: color-mix(in srgb, var(--warning) 55%, var(--line)); }
.badge.error { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 55%, var(--line)); }
.badge.ok { color: var(--ok); border-color: color-mix(in srgb, var(--ok) 55%, var(--line)); }
.gantt-shell {
  overflow-x: auto;
  border: 1px solid var(--line);
  background: var(--panel);
}
.gantt {
  min-width: 940px;
}
.gantt-row {
  display: grid;
  grid-template-columns: minmax(320px, 420px) minmax(620px, 1fr);
  border-bottom: 1px solid var(--line);
}
.gantt-row:last-child { border-bottom: 0; }
.gantt-task-cell,
.gantt-timeline {
  padding: 8px 10px;
}
.gantt-task-cell {
  border-right: 1px solid var(--line);
  overflow: hidden;
}
.gantt-resource-cell {
  color: var(--muted);
  padding-left: 28px;
}
.gantt-resource-row {
  background: color-mix(in srgb, var(--panel) 92%, var(--bg));
}
.gantt-resource-row .gantt-timeline {
  min-height: 34px;
}
.gantt-task-title {
  display: block;
  margin-bottom: 4px;
  font-weight: 700;
}
.gantt-task-ids {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin-bottom: 5px;
}
.gantt-indicators {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.gantt-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0 0 10px;
}
.legend-dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-right: 5px;
  min-height: 10px;
  padding: 0;
  vertical-align: -1px;
}
.gantt-head {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--panel);
  color: var(--muted);
  font-size: 12px;
  font-weight: 600;
}
.gantt-timeline {
  position: relative;
  display: grid;
  gap: 2px;
  align-items: center;
  min-height: 42px;
  --gantt-days: 1;
}
.gantt-timeline::before {
  content: "";
  position: absolute;
  inset: 8px 10px;
  pointer-events: none;
  z-index: 0;
  background-image: repeating-linear-gradient(to right, color-mix(in srgb, var(--line) 72%, transparent) 0 1px, transparent 1px calc(100% / var(--gantt-days)));
  -webkit-mask-image: repeating-linear-gradient(to bottom, #000 0 4px, transparent 4px 9px);
  mask-image: repeating-linear-gradient(to bottom, #000 0 4px, transparent 4px 9px);
}
.gantt-tick {
  position: relative;
  z-index: 1;
  overflow: hidden;
  color: var(--muted);
  font-size: 11px;
  text-align: center;
  white-space: nowrap;
}
.gantt-bar {
  position: relative;
  z-index: 1;
  min-height: 26px;
  padding: 5px 8px;
  border-radius: 6px;
  color: #fff;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}
.gantt-bar.status-accepted,
.gantt-bar.status-done { background: #2f8f5b; }
.gantt-bar.status-in-progress,
.gantt-bar.status-review-ready { background: #3268a8; }
.gantt-bar.status-ready,
.gantt-bar.status-not-started { background: #7b6a2b; }
.gantt-bar.status-blocked,
.gantt-bar.status-on-hold { background: #a4443d; }
.gantt-bar.status-default { background: #68726d; }
.legend-dot.gantt-bar {
  min-height: 10px;
  padding: 0;
}
.gantt-status-line {
  bottom: 4px;
  border-left: 2px solid var(--danger);
  position: absolute;
  top: 4px;
  opacity: .85;
  pointer-events: none;
  z-index: 2;
}
.gantt-allocation-bar {
  position: relative;
  z-index: 1;
  align-self: center;
  background: color-mix(in srgb, var(--bg) 76%, var(--panel));
  border: 1px solid color-mix(in srgb, var(--fg) 15%, transparent);
  border-radius: 4px;
  color: var(--fg);
  font-size: 11px;
  height: 22px;
  opacity: .8;
  overflow: hidden;
  padding: 0 5px;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
  --allocation-fill: 8%;
}
.gantt-allocation-bar::before {
  content: "";
  position: absolute;
  inset: auto 0 0;
  height: var(--allocation-fill);
  background: color-mix(in srgb, var(--ok) 58%, transparent);
  z-index: -1;
}
.gantt-allocation-bar.overloaded {
  border-color: var(--warning);
  color: var(--warning);
  opacity: 1;
}
.gantt-allocation-bar.overloaded::before {
  background: color-mix(in srgb, var(--warning) 64%, transparent);
}
.gantt-load-profile {
  position: relative;
  z-index: 1;
  align-self: stretch;
  display: grid;
  gap: 3px;
  min-height: 74px;
  padding: 6px 0;
}
.gantt-load-profile::after {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  top: var(--waterline-top, 34px);
  border-top: 1px dashed var(--warning);
  pointer-events: none;
  z-index: 3;
}
.gantt-load-bucket {
  position: relative;
  align-self: end;
  height: var(--profile-height, 56px);
  min-width: 16px;
}
.gantt-load-fill {
  position: absolute;
  bottom: 0;
  left: 2px;
  right: 2px;
  height: var(--load-height, 0%);
  background: color-mix(in srgb, var(--ok) 48%, transparent);
  border-radius: 3px 3px 0 0;
}
.gantt-load-bucket.overloaded .gantt-load-fill {
  background: color-mix(in srgb, var(--warning) 64%, transparent);
}
.gantt-load-bar {
  position: relative;
  z-index: 1;
  align-self: stretch;
  background: color-mix(in srgb, var(--ok) 45%, transparent);
  border-radius: 3px 3px 0 0;
  margin: 5px 1px 0;
}
.gantt-load-bar.overloaded {
  background: color-mix(in srgb, var(--warning) 70%, transparent);
}
ul { margin: 0; padding-left: 20px; }
li + li { margin-top: 6px; }
@media (max-width: 760px) {
  main { padding: 20px 12px 36px; }
  table { display: block; overflow-x: auto; }
  .topbar { display: block; }
  .theme-switcher { margin-top: 14px; }
  .gantt { min-width: 900px; }
  .gantt-row { grid-template-columns: 300px 1fr; }
}
</style>
<script>
(function () {
  var key = "replan-theme";
  var root = document.documentElement;
  function apply(mode) {
    if (mode === "dark" || mode === "light") {
      root.dataset.theme = mode;
    } else {
      root.removeAttribute("data-theme");
      mode = "auto";
    }
    try { localStorage.setItem(key, mode); } catch (_) {}
    document.querySelectorAll("[data-theme-choice]").forEach(function (button) {
      button.setAttribute("aria-pressed", button.dataset.themeChoice === mode ? "true" : "false");
    });
  }
  try { apply(localStorage.getItem(key) || "auto"); } catch (_) { apply("auto"); }
  window.replanSetTheme = apply;
})();
</script>
`

func WriteWeeklyHTML(w io.Writer, report WeeklyReport) error {
	if _, err := fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<title>RePlan Weekly Report %s</title>\n", html.EscapeString(report.Week)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, reportHTMLStyle, "</head>\n<body>\n<main>\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<header>\n<div class=\"topbar\"><div><h1>Weekly RePlan Report: %s</h1>\n", html.EscapeString(report.Week)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"meta\">Project: <span class=\"code\">%s</span> (%s)</div>\n", html.EscapeString(report.ProjectName), html.EscapeString(report.ProjectID)); err != nil {
		return err
	}
	if report.BaselineID != "" {
		if _, err := fmt.Fprintf(w, "<div class=\"meta\">Baseline: <span class=\"code\">%s</span></div>\n", html.EscapeString(report.BaselineID)); err != nil {
			return err
		}
	}
	if report.CurrentPlan != "" {
		if _, err := fmt.Fprintf(w, "<div class=\"meta\">Current plan: <span class=\"code\">%s</span></div>\n", html.EscapeString(report.CurrentPlan)); err != nil {
			return err
		}
	}
	if err := writeScenarioNoticeHTML(w, report.ScenarioID); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "</div>\n", themeSwitcherHTML(), "</div>\n</header>\n"); err != nil {
		return err
	}
	if err := writeSummaryHTML(w, report.Summary); err != nil {
		return err
	}
	if err := writeResourceRowsHTML(w, report.Resources); err != nil {
		return err
	}
	if err := writeIssuesHTML(w, "Errors", "error", report.Errors); err != nil {
		return err
	}
	if err := writeIssuesHTML(w, "Warnings", "warning", report.Warnings); err != nil {
		return err
	}
	if err := writeProposalsHTML(w, report.Proposals); err != nil {
		return err
	}
	if err := writeEvidenceHTML(w, report.Evidence); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "</main>\n</body>\n</html>\n")
	return err
}

func WriteDashboardHTML(w io.Writer, overview DashboardOverview) error {
	if _, err := fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<title>RePlan Dashboard: %s</title>\n", html.EscapeString(overview.ProjectName)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, reportHTMLStyle, "</head>\n<body>\n<main>\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<header>\n<div class=\"topbar\"><div><h1>RePlan Dashboard: %s</h1>\n", html.EscapeString(overview.ProjectName)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"meta\">Project: <span class=\"code\">%s</span></div>\n", html.EscapeString(overview.ProjectID)); err != nil {
		return err
	}
	if overview.StatusDate != "" {
		if _, err := fmt.Fprintf(w, "<div class=\"meta\">Status date: <span class=\"code\">%s</span></div>\n", html.EscapeString(overview.StatusDate)); err != nil {
			return err
		}
	}
	if overview.GeneratedAt != "" {
		if _, err := fmt.Fprintf(w, "<div class=\"meta\">Generated: <span class=\"code\">%s</span></div>\n", html.EscapeString(overview.GeneratedAt)); err != nil {
			return err
		}
	}
	if err := writeScenarioNoticeHTML(w, overview.ScenarioID); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, dashboardViewLinksHTML()); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "<div class=\"meta\">Raw artifacts: <a href=\"raw/overview.json\">overview.json</a> · <a href=\"raw/gantt.json\">gantt.json</a> · <a href=\"raw/openproject_overlay.json\">openproject_overlay.json</a> · <a href=\"raw/manifest.json\">manifest.json</a></div>\n</div>\n", themeSwitcherHTML(), "</div>\n</header>\n"); err != nil {
		return err
	}
	if err := writeDashboardSummaryHTML(w, overview.Health); err != nil {
		return err
	}
	if err := writeDashboardNextActionsHTML(w, overview.NextActions); err != nil {
		return err
	}
	if err := writeDashboardOpenProjectHTML(w, overview.OpenProject); err != nil {
		return err
	}
	if err := writeDashboardTrackingLoopHTML(w, overview.TrackingLoop); err != nil {
		return err
	}
	if err := writeResourceRowsHTML(w, overview.Workload); err != nil {
		return err
	}
	if err := writeDashboardTasksHTML(w, overview.Tasks); err != nil {
		return err
	}
	if err := writeDashboardDependenciesHTML(w, overview.Dependencies); err != nil {
		return err
	}
	if err := writeIssuesHTML(w, "Errors", "error", overview.Errors); err != nil {
		return err
	}
	if err := writeIssuesHTML(w, "Warnings", "warning", overview.Warnings); err != nil {
		return err
	}
	if err := writeProposalsHTML(w, overview.Proposals); err != nil {
		return err
	}
	if err := writeEvidenceHTML(w, overview.Evidence); err != nil {
		return err
	}
	if err := writeDashboardSourcesHTML(w, overview.SourceHealth); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "</main>\n</body>\n</html>\n")
	return err
}

func WriteDashboardGanttHTML(w io.Writer, overview DashboardOverview) error {
	gantt := overview.Gantt
	if _, err := fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<title>RePlan Gantt: %s</title>\n", html.EscapeString(overview.ProjectName)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, reportHTMLStyle, "</head>\n<body>\n<main>\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<header>\n<div class=\"topbar\"><div><h1>RePlan Gantt: %s</h1>\n", html.EscapeString(overview.ProjectName)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"meta\">Project: <span class=\"code\">%s</span>", html.EscapeString(overview.ProjectID)); err != nil {
		return err
	}
	if gantt.StartDate != "" || gantt.FinishDate != "" {
		if _, err := fmt.Fprintf(w, " · Range: <span class=\"code\">%s</span> - <span class=\"code\">%s</span>", html.EscapeString(gantt.StartDate), html.EscapeString(gantt.FinishDate)); err != nil {
			return err
		}
	}
	if overview.Week != "" {
		if _, err := fmt.Fprintf(w, " · Week: <span class=\"code\">%s</span>", html.EscapeString(overview.Week)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</div>\n"); err != nil {
		return err
	}
	if err := writeScenarioNoticeHTML(w, overview.ScenarioID); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, dashboardViewLinksHTML(), "<div class=\"meta\">Raw artifacts: <a href=\"raw/gantt.json\">gantt.json</a> · <a href=\"raw/overview.json\">overview.json</a> · <a href=\"raw/openproject_overlay.json\">openproject_overlay.json</a></div>\n</div>\n", themeSwitcherHTML(), "</div>\n</header>\n"); err != nil {
		return err
	}
	if err := writeDashboardGanttSummaryHTML(w, gantt); err != nil {
		return err
	}
	if err := writeDashboardOpenProjectHTML(w, overview.OpenProject); err != nil {
		return err
	}
	if err := writeDashboardGanttChartHTML(w, gantt); err != nil {
		return err
	}
	if err := writeDashboardGanttResourceLoadHTML(w, gantt); err != nil {
		return err
	}
	if err := writeDashboardGanttUnscheduledHTML(w, gantt.Unscheduled); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "</main>\n</body>\n</html>\n")
	return err
}

func dashboardViewLinksHTML() string {
	return "<div class=\"meta view-links\">Views: <a href=\"index.html\">overview</a> <a href=\"gantt.html\">gantt</a> <a href=\"workload.html\">workload</a> <a href=\"scenarios.html\">scenarios</a> <a href=\"proposals.html\">proposals</a> <a href=\"estimates.html\">estimates</a></div>\n"
}

func writeDashboardPageStart(w io.Writer, title, heading, projectID, statusDate, generatedAt, rawFile string) error {
	if _, err := fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<title>%s</title>\n", html.EscapeString(title)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, reportHTMLStyle, "</head>\n<body>\n<main>\n<header>\n<div class=\"topbar\"><div>"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<h1>%s</h1>\n<div class=\"meta\">Project: <span class=\"code\">%s</span>", html.EscapeString(heading), html.EscapeString(projectID)); err != nil {
		return err
	}
	if statusDate != "" {
		if _, err := fmt.Fprintf(w, " · Status date: <span class=\"code\">%s</span>", html.EscapeString(statusDate)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</div>\n", dashboardViewLinksHTML()); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"meta\">Raw artifact: <a href=\"raw/%s\">%s</a> · Generated: <span class=\"code\">%s</span></div>\n</div>\n%s</div>\n</header>\n", html.EscapeString(rawFile), html.EscapeString(rawFile), html.EscapeString(generatedAt), themeSwitcherHTML()); err != nil {
		return err
	}
	return nil
}

func writeDashboardPageEnd(w io.Writer) error {
	_, err := fmt.Fprint(w, "</main>\n</body>\n</html>\n")
	return err
}

func WriteDashboardWorkloadHTML(w io.Writer, page DashboardWorkloadPage) error {
	if err := writeDashboardPageStart(w, "RePlan Workload: "+page.ProjectName, "RePlan Workload: "+page.ProjectName, page.ProjectID, page.StatusDate, page.GeneratedAt, "workload.json"); err != nil {
		return err
	}
	if len(page.Data.Resources) == 0 {
		if _, err := fmt.Fprint(w, "<section><h2>Weekly workload</h2><p class=\"muted\">No workload rows.</p></section>\n"); err != nil {
			return err
		}
		return writeDashboardPageEnd(w)
	}
	visibleWeeks := dashboardWorkloadVisibleWeeks(page.Data)
	if _, err := fmt.Fprintf(w, "<section><h2>Weekly workload</h2>\n<p>Window: <span class=\"code\">%s</span> to <span class=\"code\">%s</span>. Load uses reserved amount when present, otherwise remaining amount. Utilization is load divided by capacity.</p>\n<div class=\"heat-legend\" aria-label=\"Utilization legend\"><span><i class=\"heat-swatch load-zero\"></i>0%%</span><span><i class=\"heat-swatch load-low\"></i>up to 50%%</span><span><i class=\"heat-swatch load-medium\"></i>up to 100%%</span><span><i class=\"heat-swatch load-over\"></i>over 100%%</span></div>\n<div class=\"table-scroll\">\n", html.EscapeString(page.Data.WindowStart), html.EscapeString(page.Data.WindowFinish)); err != nil {
		return err
	}
	if err := writeDashboardWorkloadHeader(w, visibleWeeks); err != nil {
		return err
	}
	for _, resource := range page.Data.Resources {
		if err := writeDashboardWorkloadRow(w, resource, visibleWeeks); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</tbody></table>\n</div>\n</section>\n"); err != nil {
		return err
	}
	return writeDashboardPageEnd(w)
}

func writeDashboardWorkloadHeader(w io.Writer, weeks []string) error {
	if _, err := fmt.Fprint(w, "<table>\n<caption class=\"sr-only\">Weekly workload heatmap</caption>\n<thead><tr><th scope=\"col\">Resource</th><th scope=\"col\">Class / unit</th>"); err != nil {
		return err
	}
	for _, week := range weeks {
		if _, err := fmt.Fprintf(w, "<th scope=\"col\">%s</th>", html.EscapeString(week)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tr></thead>\n<tbody>\n")
	return err
}

func writeDashboardWorkloadRow(w io.Writer, resource DashboardWorkloadResource, weeks []string) error {
	name := resource.ResourceName
	if name == "" {
		name = resource.ResourceID
	}
	if _, err := fmt.Fprintf(w, "<tr><th scope=\"row\"><span class=\"code\">%s</span><br>%s</th><td>%s<br><span class=\"code\">%s</span></td>", html.EscapeString(resource.ResourceID), html.EscapeString(name), html.EscapeString(resource.ResourceClass), html.EscapeString(resource.Unit)); err != nil {
		return err
	}
	byWeek := map[string]DashboardWorkloadCell{}
	for _, cell := range resource.Weeks {
		byWeek[cell.Week] = cell
	}
	for _, week := range weeks {
		cell := byWeek[week]
		if err := writeDashboardWorkloadCell(w, cell); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tr>\n")
	return err
}

func writeDashboardWorkloadCell(w io.Writer, cell DashboardWorkloadCell) error {
	class := workloadCellClass(cell.Utilization, cell.Shortage)
	load := workloadCellLoad(cell)
	if _, err := fmt.Fprintf(w, "<td class=\"%s\"><strong>%s / %s</strong>", class, formatPageNumber(load), formatPageNumber(cell.Capacity)); err != nil {
		return err
	}
	if math.Abs(cell.Remaining-load) > 0.000001 {
		if _, err := fmt.Fprintf(w, "<br><span class=\"muted\">remaining %s</span>", formatPageNumber(cell.Remaining)); err != nil {
			return err
		}
	}
	if cell.Overload > 0 {
		if _, err := fmt.Fprintf(w, "<br><span class=\"muted\">overload %s</span>", formatPageNumber(cell.Overload)); err != nil {
			return err
		}
	}
	if cell.Shortage > 0 {
		if _, err := fmt.Fprintf(w, "<br><span class=\"muted\">shortage %s</span>", formatPageNumber(cell.Shortage)); err != nil {
			return err
		}
	}
	if (cell.Overload > 0 || cell.Shortage > 0) && len(cell.Demands) > 0 {
		if _, err := fmt.Fprintf(w, "<details><summary>Demands (%d)</summary><ul>", len(cell.Demands)); err != nil {
			return err
		}
		for _, demand := range cell.Demands {
			if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span> — %s (%s), remaining %s</li>", html.EscapeString(demand.ResourceDemandID), html.EscapeString(demand.TaskTitle), html.EscapeString(demand.TaskID), formatPageNumber(demand.Remaining)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</ul></details>"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</td>")
	return err
}

func workloadCellLoad(cell DashboardWorkloadCell) float64 {
	if cell.Reserved != 0 {
		return cell.Reserved
	}
	return cell.Remaining
}

func workloadCellClass(utilization *float64, shortage float64) string {
	if shortage > 0 || (utilization != nil && *utilization > 1) {
		return "load-over"
	}
	if utilization == nil || *utilization == 0 {
		return "load-zero"
	}
	if *utilization <= 0.5 {
		return "load-low"
	}
	return "load-medium"
}

func dashboardWorkloadVisibleWeeks(data DashboardWorkloadData) []string {
	weeks := []string{}
	for _, week := range data.Weeks {
		if week >= data.WindowStart && week <= data.WindowFinish {
			weeks = append(weeks, week)
		}
	}
	return weeks
}

// formatChangeValue shows a scenario or proposal value as typed: strings and
// numbers plainly, structured values as JSON rather than Go's map syntax.
func formatChangeValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool, int, int64, float64:
		return fmt.Sprint(typed)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func formatPageNumber(value float64) string {
	return strconv.FormatFloat(math.Round(value*100)/100, 'f', -1, 64)
}

func WriteDashboardScenariosHTML(w io.Writer, page DashboardScenariosPage) error {
	if err := writeDashboardPageStart(w, "RePlan Scenarios: "+page.ProjectName, "RePlan Scenarios: "+page.ProjectName, page.ProjectID, page.StatusDate, page.GeneratedAt, "scenarios.json"); err != nil {
		return err
	}
	if len(page.Data.Scenarios) == 0 {
		if _, err := fmt.Fprint(w, "<section><h2>Scenarios</h2><p class=\"muted\">No scenarios.</p></section>\n"); err != nil {
			return err
		}
		return writeDashboardPageEnd(w)
	}
	for _, scenario := range page.Data.Scenarios {
		if err := writeDashboardScenarioHTML(w, scenario, scenario.Status == "applied" || scenario.Status == "rejected"); err != nil {
			return err
		}
	}
	return writeDashboardPageEnd(w)
}

func writeDashboardScenarioHTML(w io.Writer, scenario DashboardScenarioDetail, archived bool) error {
	if archived {
		if _, err := fmt.Fprintf(w, "<details class=\"scenario-archived\"><summary><strong>%s</strong> · <span class=\"code\">%s</span> · %s</summary><div>", html.EscapeString(scenario.Name), html.EscapeString(scenario.ID), html.EscapeString(scenario.Status)); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "<section><h2>%s</h2><p><span class=\"code\">%s</span> · %s", html.EscapeString(scenario.Name), html.EscapeString(scenario.ID), html.EscapeString(scenario.Status)); err != nil {
		return err
	}
	if !archived && scenario.CalibrationRef != "" {
		if _, err := fmt.Fprintf(w, " · calibration <span class=\"code\">%s</span>", html.EscapeString(scenario.CalibrationRef)); err != nil {
			return err
		}
	}
	if archived && scenario.CalibrationRef != "" {
		if _, err := fmt.Fprintf(w, "<p>Calibration: <span class=\"code\">%s</span></p>", html.EscapeString(scenario.CalibrationRef)); err != nil {
			return err
		}
	} else if !archived {
		if _, err := fmt.Fprint(w, "</p>"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "<table><thead><tr><th scope=\"col\">Operation</th><th scope=\"col\">Path</th><th scope=\"col\">Value</th></tr></thead><tbody>"); err != nil {
		return err
	}
	for _, change := range scenario.Changes {
		if _, err := fmt.Fprintf(w, "<tr><td>%s</td><td class=\"code\">%s</td><td class=\"code\">%s</td></tr>", html.EscapeString(change.Op), html.EscapeString(change.Path), html.EscapeString(formatChangeValue(change.Value))); err != nil {
			return err
		}
	}
	if len(scenario.Changes) == 0 {
		if _, err := fmt.Fprint(w, "<tr><td colspan=\"3\" class=\"muted\">No changes.</td></tr>"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</tbody></table><h3>Workload diff</h3><table><thead><tr><th scope=\"col\">Resource</th><th scope=\"col\">Unit</th><th scope=\"col\" class=\"num\">Capacity before</th><th scope=\"col\" class=\"num\">Capacity after</th><th scope=\"col\" class=\"num\">Overload before</th><th scope=\"col\" class=\"num\">Overload after</th></tr></thead><tbody>"); err != nil {
		return err
	}
	if err := writeScenarioDiffRows(w, scenario); err != nil {
		return err
	}
	closing := "</section>\n"
	if archived {
		closing = "</div></details>\n"
	}
	_, err := fmt.Fprintf(w, "</tbody></table><p>Preview application with this command (text only):</p><pre class=\"command code\">%s</pre>%s", html.EscapeString(scenario.ApplyCommand), closing)
	return err
}

func writeScenarioDiffRows(w io.Writer, scenario DashboardScenarioDetail) error {
	type pair struct{ before, after ResourceRow }
	rows := map[string]pair{}
	for _, row := range scenario.WorkloadBefore {
		key := row.ResourceID + "\x00" + row.Unit
		item := rows[key]
		item.before = row
		rows[key] = item
	}
	for _, row := range scenario.WorkloadAfter {
		key := row.ResourceID + "\x00" + row.Unit
		item := rows[key]
		item.after = row
		rows[key] = item
	}
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	count := 0
	for _, key := range keys {
		item := rows[key]
		if item.before.Capacity == item.after.Capacity && item.before.Remaining == item.after.Remaining && item.before.Overload == item.after.Overload {
			continue
		}
		count++
		row := item.before
		if row.ResourceID == "" {
			row = item.after
		}
		if _, err := fmt.Fprintf(w, "<tr><td class=\"code\">%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td></tr>", html.EscapeString(row.ResourceID), html.EscapeString(row.Unit), formatAmount(item.before.Capacity), formatAmount(item.after.Capacity), formatAmount(item.before.Overload), formatAmount(item.after.Overload)); err != nil {
			return err
		}
	}
	if count == 0 {
		_, err := fmt.Fprint(w, "<tr><td colspan=\"6\" class=\"muted\">No workload changes in the selected week.</td></tr>")
		return err
	}
	return nil
}

func WriteDashboardProposalsHTML(w io.Writer, page DashboardProposalsPage) error {
	if err := writeDashboardPageStart(w, "RePlan Proposals: "+page.ProjectName, "RePlan Proposals: "+page.ProjectName, page.ProjectID, page.StatusDate, page.GeneratedAt, "proposals.json"); err != nil {
		return err
	}
	if len(page.Data.Proposals) == 0 {
		if _, err := fmt.Fprint(w, "<section><h2>Open proposals</h2><p class=\"muted\">No open proposals.</p></section>\n"); err != nil {
			return err
		}
		return writeDashboardPageEnd(w)
	}
	for _, proposal := range page.Data.Proposals {
		if _, err := fmt.Fprintf(w, "<section><h2>%s</h2><p>%s · proposed by <span class=\"code\">%s</span> · target <span class=\"code\">%s</span></p><p>%s</p><table><thead><tr><th scope=\"col\">Operation</th><th scope=\"col\">Path</th><th scope=\"col\">Value</th></tr></thead><tbody><tr><td>%s</td><td class=\"code\">%s</td><td class=\"code\">%s</td></tr></tbody></table><h3>Evidence</h3>", html.EscapeString(proposal.ID), html.EscapeString(proposal.Type), html.EscapeString(proposal.CreatedBy), html.EscapeString(proposal.TargetID), html.EscapeString(proposal.Rationale), html.EscapeString(proposal.Impact.Op), html.EscapeString(proposal.Impact.Path), html.EscapeString(formatChangeValue(proposal.Impact.Value))); err != nil {
			return err
		}
		if len(proposal.Evidence) == 0 {
			if _, err := fmt.Fprint(w, "<p class=\"muted\">No related evidence.</p>"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(w, "<ul>"); err != nil {
				return err
			}
			for _, evidence := range proposal.Evidence {
				if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span> — %s</li>", html.EscapeString(evidence.ID), html.EscapeString(evidence.Summary)); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(w, "</ul>"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "<p>Run a decision with one of these commands after replacing the placeholders (text only):</p><pre class=\"command code\">%s</pre><pre class=\"command code\">%s</pre></section>\n", html.EscapeString(proposal.AcceptCommand), html.EscapeString(proposal.RejectCommand)); err != nil {
			return err
		}
	}
	return writeDashboardPageEnd(w)
}

func WriteDashboardEstimatesHTML(w io.Writer, page DashboardEstimatesPage) error {
	if err := writeDashboardPageStart(w, "RePlan Estimates: "+page.ProjectName, "RePlan Estimates: "+page.ProjectName, page.ProjectID, page.StatusDate, page.GeneratedAt, "estimates.json"); err != nil {
		return err
	}
	if len(page.Data.Baselines) == 0 {
		if _, err := fmt.Fprint(w, "<section><h2>Estimate accuracy</h2><p class=\"muted\">No sealed estimate baselines.</p></section>\n"); err != nil {
			return err
		}
		return writeDashboardPageEnd(w)
	}
	for _, baseline := range page.Data.Baselines {
		if _, err := fmt.Fprintf(w, "<section><h2>%s</h2><p><span class=\"code\">%s</span> · seal %s</p><table><thead><tr><th scope=\"col\">Resource</th><th scope=\"col\">Unit</th><th scope=\"col\" class=\"num\">Baseline</th><th scope=\"col\" class=\"num\">Actual</th><th scope=\"col\" class=\"num\">Actual / baseline</th></tr></thead><tbody>", html.EscapeString(baseline.Name), html.EscapeString(baseline.ID), html.EscapeString(baseline.SealStatus)); err != nil {
			return err
		}
		for _, resource := range baseline.Resources {
			ratio := estimateResourceRatioLabel(resource)
			if _, err := fmt.Fprintf(w, "<tr><td><span class=\"code\">%s</span><br>%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td></tr>", html.EscapeString(resource.ResourceID), html.EscapeString(resource.ResourceName), html.EscapeString(resource.Unit), formatAmount(resource.BaselineAmount), formatAmount(resource.ActualAmount), html.EscapeString(ratio)); err != nil {
				return err
			}
		}
		if len(baseline.Resources) == 0 {
			if _, err := fmt.Fprint(w, "<tr><td colspan=\"5\" class=\"muted\">No resource accuracy rows.</td></tr>"); err != nil {
				return err
			}
		}
		if err := writeDashboardEstimateSplitGroups(w, baseline.SplitGroups); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, "<h3>Calibrations using this baseline</h3>"); err != nil {
			return err
		}
		if len(baseline.Calibrations) == 0 {
			if _, err := fmt.Fprint(w, "<p class=\"muted\">No calibrations use this baseline.</p>"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprint(w, "<table><thead><tr><th scope=\"col\">Calibration</th><th scope=\"col\">Scope</th><th scope=\"col\" class=\"num\">Samples</th><th scope=\"col\" class=\"num\">Factor</th></tr></thead><tbody>"); err != nil {
				return err
			}
			for _, calibration := range baseline.Calibrations {
				if _, err := fmt.Fprintf(w, "<tr><td><span class=\"code\">%s</span><br>%s</td><td>%s · %s</td><td class=\"num\">%d</td><td class=\"num\">%s</td></tr>", html.EscapeString(calibration.ID), html.EscapeString(calibration.Name), html.EscapeString(strings.Join(calibration.Resources, ", ")), html.EscapeString(calibration.Unit), calibration.Count, formatAmount(calibration.Factor)); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(w, "</tbody></table>"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</section>\n"); err != nil {
			return err
		}
	}
	return writeDashboardPageEnd(w)
}

func estimateResourceRatioLabel(resource DashboardEstimateResource) string {
	if resource.Ratio != nil {
		return formatPageNumber(*resource.Ratio)
	}
	if resource.AddedAfterBaseline {
		return "added after baseline"
	}
	if !resource.ActualsRecorded {
		return "no actuals"
	}
	return "not comparable"
}

func writeDashboardEstimateSplitGroups(w io.Writer, groups []DashboardEstimateSplitGroup) error {
	if _, err := fmt.Fprint(w, "</tbody></table><h3>Recognized resource splits</h3>"); err != nil {
		return err
	}
	if len(groups) == 0 {
		_, err := fmt.Fprint(w, "<p class=\"muted\">No recognized resource splits.</p>")
		return err
	}
	if _, err := fmt.Fprint(w, "<table><thead><tr><th scope=\"col\">Original resource</th><th scope=\"col\">Split-out resources</th><th scope=\"col\">Unit</th><th scope=\"col\" class=\"num\">Baseline</th><th scope=\"col\" class=\"num\">Current planned</th><th scope=\"col\" class=\"num\">Actual</th><th scope=\"col\" class=\"num\">Actual / baseline</th></tr></thead><tbody>"); err != nil {
		return err
	}
	for _, group := range groups {
		ratio := "no actuals"
		if group.Ratio != nil {
			ratio = formatPageNumber(*group.Ratio)
		} else if group.ActualsRecorded {
			ratio = "not comparable"
		}
		if _, err := fmt.Fprintf(w, "<tr><td><span class=\"code\">%s</span><br>%s</td><td class=\"code\">%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td></tr>", html.EscapeString(group.OriginalResourceID), html.EscapeString(group.OriginalResourceName), html.EscapeString(strings.Join(group.SplitResourceIDs, ", ")), html.EscapeString(group.Unit), formatAmount(group.BaselineAmount), formatAmount(group.CurrentPlannedAmount), formatAmount(group.ActualAmount), html.EscapeString(ratio)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tbody></table>")
	return err
}

func writeScenarioNoticeHTML(w io.Writer, scenarioID string) error {
	if scenarioID == "" {
		return nil
	}
	_, err := fmt.Fprintf(w, "<div class=\"meta warning\"><strong>What-if: scenario %s applied in memory; this is not the plan.</strong></div>\n", html.EscapeString(scenarioID))
	return err
}

func themeSwitcherHTML() string {
	return `<div class="theme-switcher" aria-label="Theme">
<button type="button" data-theme-choice="auto" aria-pressed="true" onclick="replanSetTheme('auto')">Auto</button>
<button type="button" data-theme-choice="dark" aria-pressed="false" onclick="replanSetTheme('dark')">Dark</button>
<button type="button" data-theme-choice="light" aria-pressed="false" onclick="replanSetTheme('light')">Light</button>
</div>
`
}

func writeDashboardGanttSummaryHTML(w io.Writer, gantt DashboardGanttView) error {
	if _, err := fmt.Fprint(w, "<section aria-label=\"Gantt summary\">\n<div class=\"summary\">\n"); err != nil {
		return err
	}
	metrics := []struct {
		label string
		value int
	}{
		{"Scheduled tasks", len(gantt.Tasks)},
		{"Unscheduled tasks", len(gantt.Unscheduled)},
		{"Calendar days", gantt.DayCount},
	}
	for _, metric := range metrics {
		if _, err := fmt.Fprintf(w, "<div class=\"metric\"><span>%s</span><strong>%d</strong></div>\n", html.EscapeString(metric.label), metric.value); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</div>\n</section>\n")
	return err
}

func writeDashboardOpenProjectHTML(w io.Writer, op DashboardOpenProject) error {
	if !op.Enabled && op.LinkedTaskCount == 0 && op.MissingLinkCount == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>OpenProject Native Surface</h2>\n<div class=\"summary\">\n"); err != nil {
		return err
	}
	metrics := []struct {
		label string
		value int
	}{
		{"Linked work packages", op.LinkedTaskCount},
		{"Missing links", op.MissingLinkCount},
		{"Tasks with resources", op.TasksWithResourceCount},
		{"Work-package URLs", op.WorkPackageURLCount},
	}
	for _, metric := range metrics {
		if _, err := fmt.Fprintf(w, "<div class=\"metric\"><span>%s</span><strong>%d</strong></div>\n", html.EscapeString(metric.label), metric.value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</div>\n"); err != nil {
		return err
	}
	if op.ProjectID != "" {
		if _, err := fmt.Fprintf(w, "<div class=\"meta\">OpenProject project: <span class=\"code\">%s</span></div>\n", html.EscapeString(op.ProjectID)); err != nil {
			return err
		}
	}
	if len(op.Links) > 0 {
		if _, err := fmt.Fprint(w, "<table>\n<thead><tr><th>OpenProject</th><th>RePlan Task</th><th>Status</th><th>Dates</th><th>Resources</th><th>Warnings</th><th>Open Deps</th></tr></thead>\n<tbody>\n"); err != nil {
			return err
		}
		for _, link := range op.Links {
			opCell := "<span class=\"code\">" + html.EscapeString(link.OpenProjectID) + "</span>"
			if link.OpenProjectURL != "" {
				opCell = "<a class=\"code\" href=\"" + html.EscapeString(link.OpenProjectURL) + "\">openproject:" + html.EscapeString(link.OpenProjectID) + "</a>"
			}
			if _, err := fmt.Fprintf(w, "<tr><td>%s</td><td><strong>%s</strong><br><span class=\"code muted\">%s</span></td><td>%s</td><td><span class=\"code\">%s</span></td><td title=\"%s\">%d</td><td>%d</td><td>%d</td></tr>\n",
				opCell,
				html.EscapeString(link.Title),
				html.EscapeString(link.TaskID),
				html.EscapeString(link.TrackingStatus),
				html.EscapeString(compactDateRange(link.PlannedStart, link.PlannedFinish)),
				html.EscapeString(strings.Join(link.ResourceIDs, ", ")),
				link.ResourceDemandCount,
				link.WarningCount,
				link.OpenDependencyCount); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</tbody>\n</table>\n"); err != nil {
			return err
		}
	}
	if len(op.MissingLinks) > 0 {
		if _, err := fmt.Fprint(w, "<h2>RePlan Tasks Missing OpenProject Links</h2>\n<ul>\n"); err != nil {
			return err
		}
		for _, task := range op.MissingLinks {
			if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span> (%s): %s</li>\n",
				html.EscapeString(task.TaskID),
				html.EscapeString(task.TrackingStatus),
				html.EscapeString(task.Title)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</ul>\n"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</section>\n")
	return err
}

//nolint:gocyclo // This renderer writes one ordered HTML chart; keeping conditional fragments together makes the byte-stable document structure auditable.
func writeDashboardGanttChartHTML(w io.Writer, gantt DashboardGanttView) error {
	if len(gantt.Tasks) == 0 || gantt.DayCount == 0 {
		_, err := fmt.Fprint(w, "<section>\n<h2>Gantt</h2>\n<p class=\"muted\">No scheduled tasks with planned start and finish dates.</p>\n</section>\n")
		return err
	}
	columns := gantt.DayCount
	if columns < 1 {
		columns = 1
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Gantt</h2>\n"); err != nil {
		return err
	}
	if err := writeDashboardGanttLegendHTML(w); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "<div class=\"gantt-shell\">\n<div class=\"gantt\">\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "<div class=\"gantt-row gantt-head\"><div class=\"gantt-task-cell\">Task</div>"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"gantt-timeline\" style=\"--gantt-days: %d; grid-template-columns: repeat(%d, minmax(28px, 1fr));\">\n", columns, columns); err != nil {
		return err
	}
	for day := 0; day < columns; day++ {
		label := ganttTickLabel(gantt.StartDate, day, columns)
		if _, err := fmt.Fprintf(w, "<div class=\"gantt-tick\">%s</div>\n", html.EscapeString(label)); err != nil {
			return err
		}
	}
	if err := writeGanttStatusLineHTML(w, gantt); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "</div></div>\n"); err != nil {
		return err
	}
	for _, task := range gantt.Tasks {
		if _, err := fmt.Fprint(w, "<div class=\"gantt-row\"><div class=\"gantt-task-cell\">"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "<span class=\"gantt-task-title\">%s</span><div class=\"gantt-task-ids\"><span class=\"code muted\">%s</span>", html.EscapeString(task.Title), html.EscapeString(task.ID)); err != nil {
			return err
		}
		if task.SourceID != "" {
			sourceLabel := task.SourceID
			if task.Source != "" {
				sourceLabel = task.Source + ":" + task.SourceID
			}
			if task.SourceURL != "" {
				if _, err := fmt.Fprintf(w, "<a class=\"code\" href=\"%s\">%s</a>", html.EscapeString(task.SourceURL), html.EscapeString(sourceLabel)); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintf(w, "<span class=\"code\">%s</span>", html.EscapeString(sourceLabel)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</div><div class=\"gantt-indicators\">"); err != nil {
			return err
		}
		if _, err := writeDashboardGanttBadgesHTML(w, task); err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, "</div>"); err != nil {
			return err
		}
		start := task.StartOffsetDays + 1
		duration := task.DurationDays
		if start < 1 {
			start = 1
		}
		if duration < 1 {
			duration = 1
		}
		if _, err := fmt.Fprintf(w, "</div><div class=\"gantt-timeline\" style=\"--gantt-days: %d; grid-template-columns: repeat(%d, minmax(28px, 1fr));\">",
			columns,
			columns); err != nil {
			return err
		}
		if err := writeGanttStatusLineHTML(w, gantt); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "<div class=\"gantt-bar %s\" style=\"grid-column: %d / span %d;\" title=\"%s\">%s</div></div></div>\n",
			html.EscapeString(ganttStatusClass(task.TrackingStatus)),
			start,
			duration,
			html.EscapeString(task.PlannedStart+" - "+task.PlannedFinish),
			html.EscapeString(compactDateRange(task.PlannedStart, task.PlannedFinish))); err != nil {
			return err
		}
		for _, allocation := range task.Allocations {
			if err := writeDashboardGanttAllocationHTML(w, gantt, allocation); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprint(w, "</div>\n</div>\n</section>\n")
	return err
}

func writeDashboardGanttAllocationHTML(w io.Writer, gantt DashboardGanttView, allocation DashboardGanttResourceAllocation) error {
	columns := gantt.DayCount
	if columns < 1 {
		columns = 1
	}
	start := allocation.StartOffsetDays + 1
	duration := allocation.DurationDays
	if start < 1 {
		duration += start - 1
		start = 1
	}
	if duration < 1 {
		duration = 1
	}
	if start+duration-1 > columns {
		duration = columns - start + 1
	}
	if duration < 1 {
		return nil
	}
	className := "gantt-allocation-bar"
	if allocation.Overloaded {
		className += " overloaded"
	}
	fillPercent := 8.0
	intensityLabel := "capacity unknown"
	if allocation.CapacityKnown {
		fillPercent = allocation.IntensityPercent
		if fillPercent < 8 {
			fillPercent = 8
		}
		if fillPercent > 100 {
			fillPercent = 100
		}
		intensityLabel = formatGanttAmount(allocation.IntensityPercent) + "%"
	}
	label := intensityLabel
	if _, err := fmt.Fprintf(w, "<div class=\"gantt-row gantt-resource-row\"><div class=\"gantt-task-cell gantt-resource-cell\"><span class=\"code\">%s</span><br><span class=\"muted\">%s · %s %s</span></div>",
		html.EscapeString(allocation.ResourceID),
		html.EscapeString(allocation.Role),
		formatAmount(allocation.PlannedAmount),
		html.EscapeString(allocation.Unit)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"gantt-timeline\" style=\"--gantt-days: %d; grid-template-columns: repeat(%d, minmax(28px, 1fr));\">", columns, columns); err != nil {
		return err
	}
	if err := writeGanttStatusLineHTML(w, gantt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "<div class=\"%s\" style=\"grid-column: %d / span %d; --allocation-fill: %.1f%%;\" title=\"%s · %s - %s · window %s · planned %s %s · window capacity %s %s · intensity %s\">%s</div></div></div>\n",
		html.EscapeString(className),
		start,
		duration,
		fillPercent,
		html.EscapeString(allocation.DemandID),
		html.EscapeString(allocation.StartDate),
		html.EscapeString(allocation.FinishDate),
		html.EscapeString(allocation.ParticipationWindowSource),
		formatAmount(allocation.PlannedAmount),
		html.EscapeString(allocation.Unit),
		formatAmount(allocation.WindowCapacity),
		html.EscapeString(allocation.Unit),
		html.EscapeString(intensityLabel),
		html.EscapeString(label)); err != nil {
		return err
	}
	return nil
}

func writeDashboardGanttResourceLoadHTML(w io.Writer, gantt DashboardGanttView) error {
	if len(gantt.ResourceLoads) == 0 || gantt.DayCount == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Resource Load</h2>\n<div class=\"gantt-shell\">\n<div class=\"gantt\">\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "<div class=\"gantt-row gantt-head\"><div class=\"gantt-task-cell\">Resource</div>"); err != nil {
		return err
	}
	headerColumns := maxGanttLoadBucketCount(gantt.ResourceLoads)
	if headerColumns < 1 {
		headerColumns = 1
	}
	if _, err := fmt.Fprintf(w, "<div class=\"gantt-load-profile\" style=\"grid-template-columns: repeat(%d, minmax(44px, 1fr));\">\n", headerColumns); err != nil {
		return err
	}
	for index := 0; index < headerColumns; index++ {
		label := ""
		for _, load := range gantt.ResourceLoads {
			if index < len(load.Buckets) {
				label = load.Buckets[index].Label
				break
			}
		}
		if _, err := fmt.Fprintf(w, "<div class=\"gantt-tick\">%s</div>\n", html.EscapeString(label)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</div></div>\n"); err != nil {
		return err
	}
	for _, load := range gantt.ResourceLoads {
		name := load.ResourceName
		if name == "" {
			name = load.ResourceID
		}
		if _, err := fmt.Fprintf(w, "<div class=\"gantt-row\"><div class=\"gantt-task-cell\"><span class=\"gantt-task-title\">%s</span><span class=\"code muted\">%s</span><div class=\"gantt-indicators\"><span class=\"badge\">planned %s %s</span><span class=\"badge\">capacity %s %s/week</span>",
			html.EscapeString(name),
			html.EscapeString(load.ResourceID),
			formatAmount(load.Planned),
			html.EscapeString(load.Unit),
			formatAmount(load.Capacity),
			html.EscapeString(load.Unit)); err != nil {
			return err
		}
		if load.Overload > 0 {
			if _, err := fmt.Fprintf(w, "<span class=\"badge warning\">overload %s %s</span>", formatAmount(load.Overload), html.EscapeString(load.Unit)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</div></div>"); err != nil {
			return err
		}
		buckets := load.Buckets
		columns := len(buckets)
		if columns < 1 {
			columns = 1
		}
		maxRatio := load.MaxUtilizationRatio
		if maxRatio < 1 {
			maxRatio = 1
		}
		profileHeight := 58
		waterlineTop := profileHeight - int((1/maxRatio)*float64(profileHeight))
		if waterlineTop < 0 {
			waterlineTop = 0
		}
		if _, err := fmt.Fprintf(w, "<div class=\"gantt-load-profile\" style=\"grid-template-columns: repeat(%d, minmax(44px, 1fr)); --profile-height: %dpx; --waterline-top: %dpx;\" title=\"100%% capacity waterline; bucket type %s\">", columns, profileHeight, waterlineTop+6, html.EscapeString(load.BucketType)); err != nil {
			return err
		}
		for _, bucket := range buckets {
			heightPercent := 0.0
			if maxRatio > 0 && bucket.UtilizationRatio > 0 {
				heightPercent = (bucket.UtilizationRatio / maxRatio) * 100
			}
			if heightPercent > 100 {
				heightPercent = 100
			}
			className := "gantt-load-bucket"
			if bucket.OverCapacity {
				className += " overloaded"
			}
			if _, err := fmt.Fprintf(w, "<div class=\"%s\" title=\"%s · %s to %s · planned %s %s · capacity %s %s · utilization %s%%\"><div class=\"gantt-load-fill\" style=\"--load-height: %.1f%%;\"></div></div>",
				html.EscapeString(className),
				html.EscapeString(bucket.Label),
				html.EscapeString(bucket.StartDate),
				html.EscapeString(bucket.FinishDate),
				formatAmount(bucket.Planned),
				html.EscapeString(load.Unit),
				formatAmount(bucket.Capacity),
				html.EscapeString(load.Unit),
				formatGanttAmount(bucket.UtilizationPct),
				heightPercent); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</div></div>\n"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</div>\n</div>\n</section>\n")
	return err
}

func maxGanttLoadBucketCount(loads []DashboardGanttResourceLoad) int {
	maxCount := 0
	for _, load := range loads {
		if len(load.Buckets) > maxCount {
			maxCount = len(load.Buckets)
		}
	}
	return maxCount
}

func writeDashboardGanttLegendHTML(w io.Writer) error {
	if _, err := fmt.Fprint(w, "<div class=\"gantt-legend\">\n"); err != nil {
		return err
	}
	items := []struct {
		class string
		label string
	}{
		{"status-accepted", "accepted/done"},
		{"status-ready", "ready/not started"},
		{"status-in-progress", "in progress/review"},
		{"status-blocked", "blocked/on hold"},
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, "<span class=\"badge\"><span class=\"legend-dot gantt-bar %s\"></span>%s</span>\n", html.EscapeString(item.class), html.EscapeString(item.label)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "<span class=\"badge error\">status date</span>\n<span class=\"badge warning\">resource pressure</span>\n</div>\n"); err != nil {
		return err
	}
	return nil
}

func writeGanttStatusLineHTML(w io.Writer, gantt DashboardGanttView) error {
	if !gantt.StatusInRange || gantt.StatusDate == "" || gantt.DayCount == 0 || gantt.StatusOffset < 0 || gantt.StatusOffset >= gantt.DayCount {
		return nil
	}
	left := 0.0
	if gantt.DayCount > 1 {
		left = (float64(gantt.StatusOffset) / float64(gantt.DayCount-1)) * 100
	}
	_, err := fmt.Fprintf(w, "<div class=\"gantt-status-line\" style=\"left: %.3f%%;\" title=\"status date %s\"></div>", left, html.EscapeString(gantt.StatusDate))
	return err
}

func writeDashboardGanttBadgesHTML(w io.Writer, task DashboardGanttTask) (int64, error) {
	var written int64
	write := func(s string) error {
		n, err := io.WriteString(w, s)
		written += int64(n)
		return err
	}
	if task.TrackingStatus != "" {
		if err := write("<span class=\"badge " + html.EscapeString(ganttStatusBadgeClass(task.TrackingStatus)) + "\" title=\"tracking status\">" + html.EscapeString(task.TrackingStatus) + "</span>"); err != nil {
			return written, err
		}
	}
	if len(task.ResourceIDs) > 0 {
		if err := write(fmt.Sprintf("<span class=\"badge\" title=\"%s\">resources %d</span>", html.EscapeString(strings.Join(task.ResourceIDs, ", ")), len(task.ResourceIDs))); err != nil {
			return written, err
		}
	}
	if len(task.OverloadedResourceIDs) > 0 {
		if err := write(fmt.Sprintf("<span class=\"badge warning\" title=\"%s\">overload %d</span>", html.EscapeString(strings.Join(task.OverloadedResourceIDs, ", ")), len(task.OverloadedResourceIDs))); err != nil {
			return written, err
		}
	}
	if task.OpenDependencyCount > 0 {
		if err := write(fmt.Sprintf("<span class=\"badge warning\" title=\"open dependencies\">deps %d/%d</span>", task.OpenDependencyCount, task.DependencyCount)); err != nil {
			return written, err
		}
	}
	if len(task.WarningCodes) > 0 {
		if err := write(fmt.Sprintf("<span class=\"badge warning\" title=\"%s\">warnings %d</span>", html.EscapeString(strings.Join(task.WarningCodes, ", ")), len(task.WarningCodes))); err != nil {
			return written, err
		}
	}
	if task.HasOpenProposal {
		if err := write("<span class=\"badge warning\" title=\"open proposal\">proposal</span>"); err != nil {
			return written, err
		}
	}
	return written, nil
}

func ganttTickLabel(startDate string, day int, columns int) string {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		if day == 0 {
			return startDate
		}
		if day == columns-1 {
			return fmt.Sprintf("+%dd", day)
		}
		if day%7 == 0 {
			return fmt.Sprintf("+%dd", day)
		}
		return ""
	}
	date := start.AddDate(0, 0, day)
	if day == 0 || day == columns-1 || date.Day() == 1 {
		return date.Format("2006-01-02")
	}
	if date.Weekday() == time.Monday {
		return date.Format("01-02")
	}
	return ""
}

func compactDateRange(start string, finish string) string {
	startDate, startErr := time.Parse("2006-01-02", start)
	finishDate, finishErr := time.Parse("2006-01-02", finish)
	if startErr == nil && finishErr == nil {
		if startDate.Equal(finishDate) {
			return startDate.Format("01-02")
		}
		return startDate.Format("01-02") + " - " + finishDate.Format("01-02")
	}
	if start == finish {
		return start
	}
	return strings.TrimSpace(start + " - " + finish)
}

func formatGanttAmount(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func ganttStatusBadgeClass(status string) string {
	switch strings.ReplaceAll(status, "_", "-") {
	case "accepted", "done":
		return "ok"
	case "blocked", "on-hold":
		return "error"
	case "ready", "not-started", "in-progress", "review-ready":
		return "warning"
	default:
		return ""
	}
}

func writeDashboardGanttUnscheduledHTML(w io.Writer, tasks []DashboardGanttTask) error {
	if len(tasks) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Unscheduled Tasks</h2>\n<table>\n<thead><tr><th>Task</th><th>Status</th><th>Source</th><th>Resources</th></tr></thead>\n<tbody>\n"); err != nil {
		return err
	}
	for _, task := range tasks {
		source := task.SourceID
		if task.Source != "" && task.SourceID != "" {
			source = task.Source + ":" + task.SourceID
		}
		if _, err := fmt.Fprintf(w, "<tr><td><strong>%s</strong><br><span class=\"code muted\">%s</span></td><td>%s</td><td><span class=\"code\">%s</span></td><td><span class=\"code\">%s</span></td></tr>\n",
			html.EscapeString(task.Title),
			html.EscapeString(task.ID),
			html.EscapeString(task.TrackingStatus),
			html.EscapeString(source),
			html.EscapeString(strings.Join(task.ResourceIDs, ", "))); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tbody>\n</table>\n</section>\n")
	return err
}

func ganttStatusClass(status string) string {
	switch strings.ReplaceAll(status, "_", "-") {
	case "accepted":
		return "status-accepted"
	case "done":
		return "status-done"
	case "in-progress":
		return "status-in-progress"
	case "review-ready":
		return "status-review-ready"
	case "ready":
		return "status-ready"
	case "not-started":
		return "status-not-started"
	case "blocked":
		return "status-blocked"
	case "on-hold":
		return "status-on-hold"
	default:
		return "status-default"
	}
}

func writeSummaryHTML(w io.Writer, summary WeeklySummary) error {
	if _, err := fmt.Fprint(w, "<section aria-label=\"Summary\">\n<div class=\"summary\">\n"); err != nil {
		return err
	}
	metrics := []struct {
		label string
		value int
	}{
		{"Resource rows", summary.ResourceRows},
		{"Overloaded", summary.Overloaded},
		{"Warnings", summary.WarningCount},
		{"Errors", summary.ErrorCount},
		{"Open proposals", summary.OpenProposals},
		{"Evidence records", summary.EvidenceCount},
	}
	for _, metric := range metrics {
		if _, err := fmt.Fprintf(w, "<div class=\"metric\"><span>%s</span><strong>%d</strong></div>\n", html.EscapeString(metric.label), metric.value); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</div>\n</section>\n")
	return err
}

func writeDashboardSummaryHTML(w io.Writer, summary DashboardHealthSummary) error {
	if _, err := fmt.Fprint(w, "<section aria-label=\"Project summary\">\n<div class=\"summary\">\n"); err != nil {
		return err
	}
	metrics := []struct {
		label string
		value int
	}{
		{"Tasks", summary.TaskCount},
		{"Accepted tasks", summary.AcceptedTasks},
		{"Active tasks", summary.ActiveTasks},
		{"Open dependencies", summary.OpenDependencies},
		{"Overloaded resources", summary.OverloadedResources},
		{"Warnings", summary.WarningCount},
		{"Open proposals", summary.OpenProposals},
		{"Evidence records", summary.EvidenceCount},
	}
	for _, metric := range metrics {
		if _, err := fmt.Fprintf(w, "<div class=\"metric\"><span>%s</span><strong>%d</strong></div>\n", html.EscapeString(metric.label), metric.value); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</div>\n</section>\n")
	return err
}

func writeDashboardTrackingLoopHTML(w io.Writer, loop DashboardTrackingLoop) error {
	if loop.StaleRemainingCount == 0 && len(loop.Rows) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Tracking Proposal Loop</h2>\n<div class=\"summary\">\n"); err != nil {
		return err
	}
	metrics := []struct {
		label string
		value int
	}{
		{"Stale remaining", loop.StaleRemainingCount},
		{"Open tracking proposals", loop.OpenProposalCount},
		{"Accepted tracking proposals", loop.AcceptedProposalCount},
		{"Covered demands", loop.CoveredDemandCount},
		{"Actionable demands", loop.ActionableCount},
	}
	for _, metric := range metrics {
		if _, err := fmt.Fprintf(w, "<div class=\"metric\"><span>%s</span><strong>%d</strong></div>\n", html.EscapeString(metric.label), metric.value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "</div>\n<table>\n<thead><tr><th>Task</th><th>Demand</th><th>Status</th><th>Priority</th><th>Reason</th><th>Resource</th><th class=\"num\">Remaining</th><th class=\"num\">Forecast</th><th>Proposal</th><th>Next</th></tr></thead>\n<tbody>\n"); err != nil {
		return err
	}
	rows := loop.Rows
	if len(rows) > 20 {
		rows = rows[:20]
	}
	for _, row := range rows {
		proposal := row.ProposalID
		if proposal == "" {
			proposal = "none"
		}
		status := row.ProposalStatus
		if status == "" {
			status = "actionable"
		}
		priority := row.Priority
		if priority == "" {
			priority = "-"
		}
		reason := row.Reason
		if reason == "" {
			reason = "-"
		}
		next := row.RecommendedCommand
		if next == "" {
			next = "-"
		}
		if _, err := fmt.Fprintf(w, "<tr><td><span class=\"code\">%s</span><br><span class=\"muted\">%s</span></td><td><span class=\"code\">%s</span></td><td>%s</td><td>%s</td><td>%s</td><td><span class=\"code muted\">%s</span></td><td class=\"num\">%s</td><td class=\"num\">%s</td><td><span class=\"code\">%s</span><br><span class=\"muted\">%s</span></td><td><span class=\"code muted\">%s</span></td></tr>\n",
			html.EscapeString(row.TaskID),
			html.EscapeString(row.TaskTitle),
			html.EscapeString(row.DemandID),
			html.EscapeString(status),
			html.EscapeString(priority),
			html.EscapeString(reason),
			html.EscapeString(row.ResourceID),
			formatAmount(row.Remaining),
			formatAmount(row.Forecast),
			html.EscapeString(proposal),
			html.EscapeString(status),
			html.EscapeString(next)); err != nil {
			return err
		}
	}
	if len(loop.Rows) > len(rows) {
		if _, err := fmt.Fprintf(w, "<tr><td colspan=\"10\" class=\"muted\">%d more rows in raw/overview.json</td></tr>\n", len(loop.Rows)-len(rows)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tbody>\n</table>\n</section>\n")
	return err
}

func writeResourceRowsHTML(w io.Writer, rows []ResourceRow) error {
	if _, err := fmt.Fprint(w, "<section>\n<h2>Workload</h2>\n<table>\n<thead><tr><th>Resource</th><th>Class</th><th>Unit</th><th class=\"num\">Capacity</th><th class=\"num\">Planned</th><th class=\"num\">Reserved</th><th class=\"num\">Actual</th><th class=\"num\">Remaining</th><th class=\"num\">Overload</th></tr></thead>\n<tbody>\n"); err != nil {
		return err
	}
	for _, row := range rows {
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		className := ""
		if row.Overload > 0 {
			className = " class=\"overloaded\""
		}
		if _, err := fmt.Fprintf(w, "<tr%s><td><strong>%s</strong><br><span class=\"code muted\">%s</span></td><td>%s</td><td>%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td><td class=\"num\">%s</td></tr>\n",
			className,
			html.EscapeString(name),
			html.EscapeString(row.ResourceID),
			html.EscapeString(row.ResourceClass),
			html.EscapeString(row.Unit),
			formatAmount(row.Capacity),
			formatAmount(row.Planned),
			formatAmount(row.Reserved),
			formatAmount(row.Actual),
			formatAmount(row.Remaining),
			formatAmount(row.Overload)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tbody>\n</table>\n</section>\n")
	return err
}

func writeDashboardNextActionsHTML(w io.Writer, actions []DashboardNextAction) error {
	if len(actions) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Next Actions</h2>\n"); err != nil {
		return err
	}
	group := ""
	for i, action := range actions {
		if title := NextActionGroupTitle(action.Actor); i == 0 || title != group {
			if i > 0 {
				if _, err := fmt.Fprint(w, "</ul>\n"); err != nil {
					return err
				}
			}
			group = title
			heading := ""
			if group != "" {
				heading = "<h3>" + html.EscapeString(group) + "</h3>\n"
			}
			if _, err := fmt.Fprintf(w, "%s<ul>\n", heading); err != nil {
				return err
			}
		}
		if action.Date != "" {
			action.Label = action.Date + ": " + action.Label
		}
		target := ""
		if action.Target != "" {
			target = " <span class=\"code muted\">" + html.EscapeString(action.Target) + "</span>"
		}
		command := ""
		if action.Command != "" {
			command = "<br><span class=\"code\">" + html.EscapeString(action.Command) + "</span>"
		}
		if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span>%s: %s%s</li>\n", html.EscapeString(action.Code), target, html.EscapeString(action.Label), command); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</ul>\n</section>\n")
	return err
}

func writeDashboardTasksHTML(w io.Writer, tasks []DashboardTaskRow) error {
	if len(tasks) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Tasks</h2>\n<table>\n<thead><tr><th>Task</th><th>Status</th><th>Dates</th><th class=\"num\">Demands</th><th class=\"num\">Dependencies</th><th class=\"num\">Warnings</th></tr></thead>\n<tbody>\n"); err != nil {
		return err
	}
	for _, task := range tasks {
		dates := strings.TrimSpace(task.PlannedStart + " - " + task.PlannedFinish)
		if dates == "-" {
			dates = ""
		}
		if _, err := fmt.Fprintf(w, "<tr><td><strong>%s</strong><br><span class=\"code muted\">%s</span></td><td>%s</td><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">%d</td></tr>\n",
			html.EscapeString(task.Title),
			html.EscapeString(task.ID),
			html.EscapeString(task.TrackingStatus),
			html.EscapeString(dates),
			task.ResourceDemandCount,
			task.DependencyCount,
			task.WarningCount); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tbody>\n</table>\n</section>\n")
	return err
}

func writeDashboardDependenciesHTML(w io.Writer, dependencies []DashboardDependency) error {
	if len(dependencies) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Dependencies</h2>\n<table>\n<thead><tr><th>Dependency</th><th>Predecessor</th><th>Successor</th><th>Status</th><th>Criticality</th></tr></thead>\n<tbody>\n"); err != nil {
		return err
	}
	for _, dependency := range dependencies {
		if _, err := fmt.Fprintf(w, "<tr><td><span class=\"code\">%s</span><br>%s</td><td><span class=\"code\">%s</span></td><td><span class=\"code\">%s</span></td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(dependency.ID),
			html.EscapeString(dependency.Type),
			html.EscapeString(dependency.PredecessorID),
			html.EscapeString(dependency.SuccessorID),
			html.EscapeString(dependency.Status),
			html.EscapeString(dependency.Criticality)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</tbody>\n</table>\n</section>\n")
	return err
}

func writeIssuesHTML(w io.Writer, title string, className string, issues []ReportIssue) error {
	if len(issues) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "<section>\n<h2>%s</h2>\n<ul>\n", html.EscapeString(title)); err != nil {
		return err
	}
	for _, issue := range issues {
		target := ""
		if issue.TargetID != "" {
			target = " <span class=\"code muted\">" + html.EscapeString(issue.TargetID) + "</span>"
		}
		if _, err := fmt.Fprintf(w, "<li><span class=\"code %s\">%s</span>%s: %s</li>\n", html.EscapeString(className), html.EscapeString(issue.Code), target, html.EscapeString(issue.Message)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</ul>\n</section>\n")
	return err
}

func writeProposalsHTML(w io.Writer, proposals []ProposalRow) error {
	if len(proposals) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Open Proposals</h2>\n<ul>\n"); err != nil {
		return err
	}
	for _, proposal := range proposals {
		if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span> (%s): %s</li>\n", html.EscapeString(proposal.ID), html.EscapeString(proposal.Type), html.EscapeString(proposal.Rationale)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</ul>\n</section>\n")
	return err
}

func writeEvidenceHTML(w io.Writer, evidence []EvidenceRow) error {
	if len(evidence) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Evidence</h2>\n<ul>\n"); err != nil {
		return err
	}
	for _, item := range evidence {
		if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span> (%s): %s</li>\n", html.EscapeString(item.ID), html.EscapeString(item.Type), html.EscapeString(item.Summary)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</ul>\n</section>\n")
	return err
}

func writeDashboardSourcesHTML(w io.Writer, health DashboardSourceHealth) error {
	if len(health.Profiles) == 0 && health.SourceRefs == 0 && len(health.LinkedTasks) == 0 && len(health.UnlinkedTasks) == 0 {
		return nil
	}
	if _, err := fmt.Fprint(w, "<section>\n<h2>Source Health</h2>\n<div class=\"meta\">"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Project source refs: <span class=\"code\">%d</span>; task source refs: <span class=\"code\">%d</span>; resource source refs: <span class=\"code\">%d</span>; dependency source refs: <span class=\"code\">%d</span>.",
		health.SourceRefs,
		health.TasksWithSourceRefs,
		health.ResourcesWithSourceRefs,
		health.DependenciesWithSourceRefs); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "</div>\n"); err != nil {
		return err
	}
	if len(health.Profiles) > 0 {
		if _, err := fmt.Fprint(w, "<table>\n<thead><tr><th>Profile</th><th>Source</th><th>Project</th><th>Base URL</th></tr></thead>\n<tbody>\n"); err != nil {
			return err
		}
		for _, profile := range health.Profiles {
			if _, err := fmt.Fprintf(w, "<tr><td><span class=\"code\">%s</span></td><td>%s</td><td><span class=\"code\">%s</span></td><td><span class=\"code\">%s</span></td></tr>\n",
				html.EscapeString(profile.ID),
				html.EscapeString(profile.Source),
				html.EscapeString(profile.ProjectID),
				html.EscapeString(profile.BaseURL)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</tbody>\n</table>\n"); err != nil {
			return err
		}
	}
	if len(health.LinkedTasks) > 0 {
		if _, err := fmt.Fprint(w, "<h2>OpenProject Links</h2>\n<table>\n<thead><tr><th>Task</th><th>Status</th><th>Source</th><th>Source ID</th><th>URL</th></tr></thead>\n<tbody>\n"); err != nil {
			return err
		}
		for _, task := range health.LinkedTasks {
			if _, err := fmt.Fprintf(w, "<tr><td><strong>%s</strong><br><span class=\"code muted\">%s</span></td><td>%s</td><td>%s</td><td><span class=\"code\">%s</span></td><td><span class=\"code\">%s</span></td></tr>\n",
				html.EscapeString(task.Title),
				html.EscapeString(task.TaskID),
				html.EscapeString(task.TrackingStatus),
				html.EscapeString(task.Source),
				html.EscapeString(task.SourceID),
				html.EscapeString(task.SourceURL)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</tbody>\n</table>\n"); err != nil {
			return err
		}
	}
	if len(health.UnlinkedTasks) > 0 {
		if _, err := fmt.Fprint(w, "<h2>Tasks Without Source Links</h2>\n<ul>\n"); err != nil {
			return err
		}
		for _, task := range health.UnlinkedTasks {
			if _, err := fmt.Fprintf(w, "<li><span class=\"code\">%s</span> (%s): %s</li>\n",
				html.EscapeString(task.TaskID),
				html.EscapeString(task.TrackingStatus),
				html.EscapeString(task.Title)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "</ul>\n"); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "</section>\n")
	return err
}
