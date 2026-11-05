package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

func runDemand(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing demand subcommand")
		return exitInvalidUsage
	}
	switch args[0] {
	case "add":
		return runDemandAdd(args[1:], stdout, stderr)
	case "set":
		return runDemandSet(args[1:], stdout, stderr)
	case "move":
		return runDemandMove(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown demand subcommand: %s\n", args[0])
		return exitInvalidUsage
	}
}

// Explicit tags keep the batch shape identical to long flags with underscores.
type demandAddBatchDocument struct {
	Demands []demandAddBatchItem `yaml:"demands"`
}
type demandAddBatchItem struct {
	ID            string  `yaml:"id"`
	Suffix        string  `yaml:"suffix"`
	Task          string  `yaml:"task"`
	Resource      string  `yaml:"resource"`
	Pool          string  `yaml:"pool"`
	ResourceClass string  `yaml:"resource_class"`
	Role          string  `yaml:"role"`
	Unit          string  `yaml:"unit"`
	Amount        float64 `yaml:"amount"`
	Criticality   string  `yaml:"criticality"`
	Status        string  `yaml:"status"`
	EarliestStart string  `yaml:"earliest_start"`
	LatestFinish  string  `yaml:"latest_finish"`
}

type demandSetBatchDocument struct {
	Demands []demandSetBatchItem `yaml:"demands"`
}
type demandSetBatchItem struct {
	ID            string   `yaml:"id"`
	Amount        *float64 `yaml:"amount"`
	Remaining     *float64 `yaml:"remaining"`
	Unit          *string  `yaml:"unit"`
	Role          *string  `yaml:"role"`
	Criticality   *string  `yaml:"criticality"`
	Status        *string  `yaml:"status"`
	EarliestStart *string  `yaml:"earliest_start"`
	LatestFinish  *string  `yaml:"latest_finish"`
}

func runDemandAdd(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("demand add")
	projectPath, format := addProjectFormatFlags(fs, "json")
	id := fs.String("id", "", "optional full demand id")
	suffix := fs.String("suffix", "", "suffix used to derive the demand id")
	task := fs.String("task", "", "existing task id")
	resource := fs.String("resource", "", "target resource id")
	pool := fs.String("pool", "", "target resource pool id")
	resourceClass := fs.String("resource-class", "", "resource class; defaults from the target")
	role := fs.String("role", "", "demand role; defaults to suffix")
	unit := fs.String("unit", "", "capacity unit")
	amount := fs.Float64("amount", 0, "planned amount")
	criticality := fs.String("criticality", "required", "required, preferred, optional or substitute")
	status := fs.String("status", "active", "demand status")
	earliest := fs.String("earliest-start", "", "optional YYYY-MM-DD")
	latest := fs.String("latest-finish", "", "optional YYYY-MM-DD")
	fromFile := fs.String("from-file", "", "YAML file containing a demands list")
	reason, actor, evidence, occurredAt := demandAuditFlags(fs)
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("demand add", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("demand add", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	items := []app.DemandAddItem{{ID: *id, Suffix: *suffix, TaskID: *task, ResourceID: *resource, PoolID: *pool, ResourceClass: *resourceClass, Role: *role, Unit: *unit, Amount: *amount, Criticality: *criticality, Status: *status, EarliestStart: *earliest, LatestFinish: *latest}}
	if *fromFile != "" {
		var batch demandAddBatchDocument
		if err := readYAMLBatch(*fromFile, &batch); err != nil {
			return printInputFileError("demand add", *projectPath, *format, *fromFile, err, stdout, stderr)
		}
		items = make([]app.DemandAddItem, 0, len(batch.Demands))
		for _, item := range batch.Demands {
			items = append(items, app.DemandAddItem{ID: item.ID, Suffix: item.Suffix, TaskID: item.Task, ResourceID: item.Resource, PoolID: item.Pool, ResourceClass: item.ResourceClass, Role: item.Role, Unit: item.Unit, Amount: item.Amount, Criticality: item.Criticality, Status: item.Status, EarliestStart: item.EarliestStart, LatestFinish: item.LatestFinish})
		}
	}
	result, err := app.NewDemandService(newStore(*projectPath)).Add(context.Background(), app.DemandAddRequest{Items: items, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	return printDemandResult("demand add", *projectPath, *format, result, err, stdout, stderr)
}

func runDemandSet(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("demand set")
	projectPath, format := addProjectFormatFlags(fs, "json")
	amount := fs.Float64("amount", 0, "new planned amount")
	remaining := fs.Float64("remaining", 0, "new remaining amount")
	unit := fs.String("unit", "", "new capacity unit")
	role := fs.String("role", "", "new role")
	criticality := fs.String("criticality", "", "new criticality")
	status := fs.String("status", "", "new status")
	earliest := fs.String("earliest-start", "", "new earliest start YYYY-MM-DD")
	latest := fs.String("latest-finish", "", "new latest finish YYYY-MM-DD")
	fromFile := fs.String("from-file", "", "YAML file containing a demands list")
	reason, actor, evidence, occurredAt := demandAuditFlags(fs)
	writeFlags := addCanonicalWriteFlags(fs)
	positional, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positional = positionalArg(positional, fs)
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("demand set", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("demand set", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	set := visitedFlags(fs)
	item := app.DemandSetItem{ID: positional, Amount: floatPtrIf(set["amount"], *amount), Remaining: floatPtrIf(set["remaining"], *remaining), Unit: stringPtrIf(set["unit"], *unit), Role: stringPtrIf(set["role"], *role), Criticality: stringPtrIf(set["criticality"], *criticality), Status: stringPtrIf(set["status"], *status), EarliestStart: stringPtrIf(set["earliest-start"], *earliest), LatestFinish: stringPtrIf(set["latest-finish"], *latest)}
	items := []app.DemandSetItem{item}
	if *fromFile != "" {
		var batch demandSetBatchDocument
		if err := readYAMLBatch(*fromFile, &batch); err != nil {
			return printInputFileError("demand set", *projectPath, *format, *fromFile, err, stdout, stderr)
		}
		items = make([]app.DemandSetItem, 0, len(batch.Demands))
		for _, item := range batch.Demands {
			items = append(items, app.DemandSetItem{ID: item.ID, Amount: item.Amount, Remaining: item.Remaining, Unit: item.Unit, Role: item.Role, Criticality: item.Criticality, Status: item.Status, EarliestStart: item.EarliestStart, LatestFinish: item.LatestFinish})
		}
	}
	result, err := app.NewDemandService(newStore(*projectPath)).Set(context.Background(), app.DemandSetRequest{Items: items, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	return printDemandResult("demand set", *projectPath, *format, result, err, stdout, stderr)
}

func runDemandMove(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("demand move")
	projectPath, format := addProjectFormatFlags(fs, "json")
	id := fs.String("id", "", "optional full id for the new demand")
	suffix := fs.String("suffix", "", "suffix used to derive the new demand id")
	toResource := fs.String("to-resource", "", "target resource id")
	toPool := fs.String("to-pool", "", "target resource pool id")
	amount := fs.Float64("amount", 0, "new planned amount; defaults to old remaining")
	reason, actor, evidence, occurredAt := demandAuditFlags(fs)
	writeFlags := addCanonicalWriteFlags(fs)
	positional, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positional = positionalArg(positional, fs)
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("demand move", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("demand move", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	set := visitedFlags(fs)
	result, err := app.NewDemandService(newStore(*projectPath)).Move(context.Background(), app.DemandMoveRequest{ID: positional, NewID: *id, Suffix: *suffix, ToResourceID: *toResource, ToPoolID: *toPool, Amount: floatPtrIf(set["amount"], *amount), Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	return printDemandResult("demand move", *projectPath, *format, result, err, stdout, stderr)
}

func demandAuditFlags(fs *flag.FlagSet) (reason, actor, evidence, occurredAt *string) {
	return fs.String("reason", "", "required audit reason"), fs.String("actor", "", "required existing audit actor"), fs.String("evidence", "", "required existing evidence id shared by the batch"), fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset")
}
func visitedFlags(fs *flag.FlagSet) map[string]bool {
	out := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { out[f.Name] = true })
	return out
}
func floatPtrIf(ok bool, value float64) *float64 {
	if !ok {
		return nil
	}
	return &value
}
func stringPtrIf(ok bool, value string) *string {
	if !ok {
		return nil
	}
	return &value
}
func readYAMLBatch(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, target)
}
func printDemandResult(command, projectPath, format string, result app.DemandResult, err error, stdout, stderr *os.File) int {
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: command, ProjectPath: projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(format, "text") && err == nil {
		return writeAddItemsText(stdout, "Demands", result.DryRun, result.Applied, demandIDs(result.Demands))
	}
	return printScenarioEnvelope(stdout, stderr, format, env, err)
}
func demandIDs(items []domain.ResourceDemand) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
