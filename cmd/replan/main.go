package main

import (
	"fmt"
	"os"

	"github.com/kogriv/replan/internal/storage/filestore"
)

const (
	exitSuccess          = 0
	exitGeneralError     = 1
	exitValidationFailed = 2
	exitPolicyDenied     = 3
	exitNotFound         = 4
	exitConflict         = 5
	exitExternalSource   = 6
	exitInvalidUsage     = 8
	bindingsOutputCode   = "bindings_layer_output_in_project"
)

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(args []string, stdout, stderr *os.File) (code int) {
	originalBindings, hadBindings := os.LookupEnv("REPLAN_OPENPROJECT_BINDINGS_LAYER")
	defer func() {
		if hadBindings {
			_ = os.Setenv("REPLAN_OPENPROJECT_BINDINGS_LAYER", originalBindings)
		} else {
			_ = os.Unsetenv("REPLAN_OPENPROJECT_BINDINGS_LAYER")
		}
	}()
	intent := projectWriteIntentForArgs(args)
	if intent.write && !intent.bindingsLayer {
		lock, err := acquireProjectWriteLock(intent.projectPath, projectWriteLockTimeout())
		if err != nil {
			return printProjectLocked(stdout, stderr, intent, err)
		}
		if lock != nil {
			defer lock.close()
		}
	}
	code = runCommand(args, stdout, stderr)
	if code != exitSuccess {
		if path := os.Getenv("REPLAN_OPENPROJECT_BINDINGS_LAYER"); path != "" && filestore.OpenProjectBindingsError(path) != nil {
			return exitInvalidUsage
		}
	}
	return code
}

func runCommand(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidUsage
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr, true)
	case "--version":
		return runVersion(args[1:], stdout, stderr, false)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "workload":
		return runWorkload(args[1:], stdout, stderr)
	case "report":
		return runReport(args[1:], stdout, stderr)
	case "next":
		return runNext(args[1:], stdout, stderr)
	case "project":
		return runProject(args[1:], stdout, stderr)
	case "tracking":
		return runTracking(args[1:], stdout, stderr)
	case "scenario":
		return runScenario(args[1:], stdout, stderr)
	case "proposal":
		return runProposal(args[1:], stdout, stderr)
	case "dependency":
		return runDependency(args[1:], stdout, stderr)
	case "baseline":
		return runBaseline(args[1:], stdout, stderr)
	case "estimate":
		return runEstimate(args[1:], stdout, stderr)
	case "audit":
		return runAudit(args[1:], stdout, stderr)
	case "actual":
		return runActual(args[1:], stdout, stderr)
	case "evidence":
		return runEvidence(args[1:], stdout, stderr)
	case "task":
		return runTask(args[1:], stdout, stderr)
	case "resource":
		return runResource(args[1:], stdout, stderr)
	case "capacity":
		return runCapacity(args[1:], stdout, stderr)
	case "import":
		return runImport(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "taskjuggler":
		return runTaskJuggler(args[1:], stdout, stderr)
	case "dev":
		return runDev(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func printUsage(out *os.File) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  Workspace-loading commands accept [--bindings <path>] (or REPLAN_OPENPROJECT_BINDINGS_LAYER; flag wins).")
	fmt.Fprintln(out, "  replan version [--format text|json]")
	fmt.Fprintln(out, "  replan init <dir> --name <name> --owner <display-name> [--id <project-id>] [--timezone <IANA-zone>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan validate [project-path] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan workload --project <path> [--week <YYYY-Www>] [--resource <id>] [--resource-class <class>] [--unit <unit>] [--scenario <id>] [--demands] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan report weekly --project <path> --week <YYYY-Www> [--format json|markdown|csv|html] [--write]")
	fmt.Fprintln(out, "  replan report estimates --project <path> --baseline <id> [--format json|markdown]")
	fmt.Fprintln(out, "  replan estimate calibrate --project <path> --baseline <id> [--baseline <id> ...] --resource <id> [--resource <id> ...] --unit <unit> [--tag <tag>] [--exclude <task-id>=<reason> ...] --id <id> --name <name> --approved-by <actor-id> (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan estimate list --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan estimate show --project <path> <id> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan estimate apply --project <path> --calibration <id> --scenario-id <id> --actor <id> (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan next --project <path> [--week <YYYY-Www>] [--format text|json]")
	fmt.Fprintln(out, "  replan project migrate --project <path> --actor <id> [--reason <text>] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan project set-status-date --project <path> --date <YYYY-MM-DD|today> --actor <id> [--reason <text>] (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan report state --project <path> [--week <YYYY-Www>] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan report dashboard --project <path> [--week <YYYY-Www>] [--output <dir>] [--format html|json]")
	fmt.Fprintln(out, "  replan tracking suggest --project <path> [--week <YYYY-Www>] [--write-candidates --actor <id>] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan scenario list|show|diff|apply --project <path> [--dry-run|--apply for apply] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan proposal list|show|suggest-dependencies|accept|reject --project <path> [--dry-run|--apply for accept/reject] [--format json|markdown]")
	fmt.Fprintln(out, "  replan dependency list --project <path> [--format json|markdown]")
	fmt.Fprintln(out, "  replan baseline create --project <path> --id <id> --name <name> --approved-by <actor-id> [--status-date <YYYY-MM-DD>] [--assumption <text> ...] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan baseline list --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan baseline show <id> --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan audit list|verify --project <path> [--format json|markdown]")
	fmt.Fprintln(out, "  replan actual list|add-consumption|set-remaining|close-demand|reopen-demand|acknowledge-overrun|add-status --project <path> [--dry-run|--apply for mutations] [--format json|markdown]")
	fmt.Fprintln(out, "  replan evidence add --project <path> --id <id> --type <type> [--subtype <subtype>] [--source <source>] [--url <url>] --actor <id> --observed-at <RFC3339> [--confidence <confidence>] --summary <text> --target <kind:id> [--target <kind:id> ...] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan resource add --project <path> --id <resource-id> --class <class> --name <name> [--capability <name> ...] [--with-actor <trust-level>] --actor <id> [--evidence <id>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan resource list --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan capacity set --project <path> --resource <resource-id> --unit <unit> --amount <n> [--period week] [--effective-from <YYYY-MM-DD>] --actor <id> [--evidence <id>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan task add --project <path> --id <task-id> --title <title> [--description <text>] [--type <type>] [--planned-start <YYYY-MM-DD>] [--planned-finish <YYYY-MM-DD>] [--tag <tag> ...] [--assignee <resource-id>] [--after <task-id>] --demand <SUFFIX=RESOURCE_ID:AMOUNT:UNIT[:ROLE]> [--demand ...] --actor <id> --evidence <id> [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text] (does not create an OpenProject work package; next use scripts/apply-openproject-bootstrap.sh or toolkit work-package create + bind)")
	fmt.Fprintln(out, "  replan task set-dates --project <path> --task <task-id> [--planned-start <YYYY-MM-DD>] [--planned-finish <YYYY-MM-DD>] --reason <text> --actor <id> --evidence <id> [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text] (push to OpenProject separately with scripts/push-openproject-date-drift.sh)")
	fmt.Fprintln(out, "  replan task record-actual-window --project <path> --task <task-id> --actor <id> --evidence <id> [--reason <text>] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan task set-title --project <path> --task <task-id> --title <title> --reason <text> --actor <id> --evidence <id> [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan import github --project <path> (--from-file <path>|--profile <id>|--repository <owner/repo>) [--token-env <env>] [--base-url <url>] [--since <date>] [--include-closed] [--include-relations] [--resource-links-sidecar <path>] [--resource-demands-sidecar <path>] [--request-timeout <seconds>] [--request-retries <n>] [--request-retry-delay <seconds>] [--dry-run] [--format json|markdown]")
	fmt.Fprintln(out, "  replan import openproject --project <path> (--from-file <path>|--profile <id>|--base-url <url> --project-id <id> --token-env <env>) [--work-package <id>] [--since <date>] [--include-closed] [--include-relations] [--include-time-entries] [--resource-demands-sidecar <path>] [--resource-links-sidecar <path>] [--page-size <n>] [--request-timeout <seconds>] [--request-retries <n>] [--request-retry-delay <seconds>] [--host-header <host>] [--forwarded-proto <proto>] [--perimeter-key-env <env>] [--perimeter-header <name>] [--dry-run] [--format json|markdown]")
	fmt.Fprintln(out, "  replan import accept <source-snapshot-id> --project <path> (--dry-run|--apply --actor <id> [--reason <text>]) [--accept-dates|--accept-dependencies|--accept-tracking-status|--accept-resource-links|--accept-resource-demands|--accept-time-entries] [--format json|markdown] (tracker candidates; time entries currently require an OpenProject snapshot)")
	fmt.Fprintln(out, "  replan export json --project <path> [--output <path>] [--manifest <path>] [--format json]")
	fmt.Fprintln(out, "  replan export taskjuggler --project <path> [--scenario <id>] [--output <path>] [--manifest <path>] [--format text|json] [--unrepresented-assumptions warn|error]")
	fmt.Fprintln(out, "  replan export csv --project <path> --report workload --week <YYYY-Www> [--scenario <id>] [--output <path>] [--manifest <path>]")
	fmt.Fprintln(out, "  replan taskjuggler run --project <path> [--scenario <id>] [--output-dir <path>] [--taskjuggler-bin tj3] [--unrepresented-assumptions warn|error] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan taskjuggler import-result --project <path> --run-dir <path> (--dry-run|--write-candidate --actor <id>) [--scenario-id <id>] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan dev validate-taskjuggler --project <path> [--scenario <id>] [--taskjuggler-bin tj3] [--keep-temp] [--format json|markdown|text]")
}
