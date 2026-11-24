package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"opsagi/pkg/config"
	"opsagi/pkg/opsremote"
)

func TestCatalogueArgs_CarriesEveryPlaceholderTheCatalogueDeclares(t *testing.T) {
	args := catalogueArgs(OpsExecArgs{
		Command:   "image_history",
		Unit:      "docker.service",
		Container: "caddy",
		Path:      "/etc/nginx/nginx.conf",
		Image:     "caddy:2.10.2-alpine",
		Lines:     50,
	})

	rendered, err := opsremote.Render("image_history", args)
	if err != nil {
		t.Fatalf("an argument the schema carries was not rendered: %v", err)
	}
	if !strings.Contains(rendered, "caddy:2.10.2-alpine") || !strings.Contains(rendered, "-n 50") {
		t.Errorf("command = %q, want the image and the count in it", rendered)
	}
}

func TestCatalogueArgs_LeavesTheCountOutWhenItWasNotGiven(t *testing.T) {
	args := catalogueArgs(OpsExecArgs{Command: "listen_ports"})
	if _, ok := args["Lines"]; ok {
		t.Errorf("Lines = %q, want the catalogue to apply its own default", args["Lines"])
	}
}

func TestOpsTools_HandleExec_RefusesAnUnknownCommandBeforeItDials(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}

	_, err := tools.HandleExec(t.Context(), OpsExecToolName, []byte(`{"command":"run_rm_rf"}`))
	if err == nil {
		t.Fatal("unknown command was accepted")
	}
	if !strings.Contains(err.Error(), "未知的只读命令") {
		t.Errorf("error = %q, want the catalogue refusal", err)
	}
}

func TestOpsTools_HandleExec_RefusesAnArgumentCarryingAShellBeforeItDials(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}

	_, err := tools.HandleExec(t.Context(), OpsExecToolName, []byte(`{"command":"unit_status","unit":"sshd; id"}`))
	if err == nil {
		t.Fatal("an argument carrying a shell was accepted")
	}
	if !strings.Contains(err.Error(), "非法") {
		t.Errorf("error = %q, want the argument refusal", err)
	}
}

func TestOpsTools_HandleSandbox_RefusesWorkshopWithoutEgressAsAnAnswer(t *testing.T) {
	tools := &opsTools{
		targets:        map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}},
		sandboxNetwork: false,
	}

	result, err := tools.HandleSandbox(t.Context(), OpsSandboxToolName, []byte(`{"target":"lab","script":"echo hi","mode":"workshop"}`))
	if err == nil {
		t.Fatalf("workshop without egress was allowed, result = %q", result)
	}

	reason, refused := refusalText(err)
	if !refused {
		t.Fatalf("error %q is not a refusal, so the model reads it as a broken tool", err)
	}
	for _, want := range []string{"OPSAGI_SANDBOX_NETWORK", "ask"} {
		if !strings.Contains(reason, want) {
			t.Errorf("refusal %q does not mention %q", reason, want)
		}
	}
}

func TestOpsTools_HandleSandbox_RefusesAnImageNameCarryingShellAsAnAnswer(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}, sandboxNetwork: true}

	_, err := tools.HandleSandbox(t.Context(), OpsSandboxToolName, []byte(`{"target":"lab","script":"echo hi","image":"debian;reboot"}`))
	if err == nil {
		t.Fatal("an image name carrying a shell was accepted")
	}
	if _, refused := refusalText(err); !refused {
		t.Fatalf("error %q is not a refusal", err)
	}
}

func TestOpsTools_HandleSandbox_RefusesAnUnknownShapeBeforeItDials(t *testing.T) {
	// No targets: if the shape were checked after dialling, the answer would be
	// "no host configured" rather than the shape that the model can correct.
	tools := &opsTools{sandboxNetwork: true}

	_, err := tools.HandleSandbox(t.Context(), OpsSandboxToolName, []byte(`{"target":"lab","script":"echo hi","mode":"root"}`))
	if err == nil {
		t.Fatal("an unknown shape was accepted")
	}

	reason, refused := refusalText(err)
	if !refused {
		t.Fatalf("error %q is not a refusal, so the model reads it as a broken tool", err)
	}
	for _, want := range []string{"root", "probe", "workshop"} {
		if !strings.Contains(reason, want) {
			t.Errorf("refusal %q does not mention %q", reason, want)
		}
	}
}

func TestOpsTools_HandleExec_AnswersAnEmptyCommandWithTheCatalogue(t *testing.T) {
	tools := &opsTools{}

	_, err := tools.HandleExec(t.Context(), OpsExecToolName, []byte(`{}`))
	if err == nil {
		t.Fatal("a missing command was accepted")
	}
	for _, want := range []string{"缺少 command", "listen_ports", "docker_inspect"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestFormatExec_QuotesTheCommandThatProducedTheEvidence(t *testing.T) {
	target := opsremote.Target{ID: "lab", Host: "192.0.2.10", Port: 22}
	res := opsremote.Result{
		Command:  "ss -lntup 2>/dev/null",
		Stdout:   "State  Local\nLISTEN 0.0.0.0:443\n",
		ExitCode: 0,
	}

	out := formatExec(target, "listen_ports", res)

	for _, want := range []string{
		"192.0.2.10:22",
		"listen_ports",
		"退出码 0",
		"ss -lntup 2>/dev/null",
		"0.0.0.0:443",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
}

func TestFormatExec_SaysWhenTheCommandProducedNothing(t *testing.T) {
	target := opsremote.Target{ID: "lab", Host: "192.0.2.10", Port: 22}

	out := formatExec(target, "kernel_log", opsremote.Result{Command: "dmesg -T", ExitCode: 1})

	if !strings.Contains(out, "没有任何输出") {
		t.Errorf("an empty result reads as evidence of absence:\n%s", out)
	}
}

// A param the renderer knows but the tool has no field for reads to the model as
// a missing argument it cannot supply, so the command is unusable however well it
// is written.
func TestOpsTools_HandleExec_GivesTheModelAFieldForEveryCatalogueParam(t *testing.T) {
	t.Parallel()

	filled := OpsExecArgs{
		Target:    "lab",
		Command:   "listen_ports",
		Unit:      "nginx.service",
		Container: "opslab-broken",
		Path:      "/etc/nginx/nginx.conf",
		Image:     "nginx:1.27-alpine",
		Database:  "shop",
		Lines:     10,
		Rank:      3,
	}

	available := catalogueArgs(filled)

	for _, command := range opsremote.Commands() {
		for _, param := range command.Params {
			if _, ok := available[param.Template]; !ok {
				t.Errorf("command %q declares param %q, which the tool cannot supply", command.ID, param.Template)
			}
		}
	}
}

func TestOpsTools_TraceIsAbsentUntilTheOperatorTurnsItOn(t *testing.T) {
	host := config.Config{OpsTargets: `[{"id":"lab","host":"192.0.2.10"}]`}

	off := NewOpsTools(&host)
	if off.IsAvailable() != true {
		t.Fatal("the fixture has no target, so this proves nothing about the gate")
	}
	if off.TraceAvailable() {
		t.Error("kernel tracing is offered before the operator allowed it")
	}

	host.OpsSandboxBPF = true
	if !NewOpsTools(&host).TraceAvailable() {
		t.Error("kernel tracing is still absent with OPSAGI_SANDBOX_BPF set")
	}

	noHost := config.Config{OpsSandboxBPF: true}
	if NewOpsTools(&noHost).TraceAvailable() {
		t.Error("kernel tracing is offered without a target to trace")
	}
}

func TestOpsTools_HandleTrace_RefusesAnIdOutsideTheCatalogueAsAnAnswer(t *testing.T) {
	tools := &opsTools{
		targets:    map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}},
		sandboxBPF: true,
	}

	_, err := tools.HandleTrace(t.Context(), OpsTraceToolName, []byte(`{"target":"lab","script":"run_rm_rf"}`))
	if err == nil {
		t.Fatal("an id outside the catalogue was accepted")
	}

	reason, refused := refusalText(err)
	if !refused {
		t.Fatalf("error %q is not a refusal, so the model reads it as a broken tool", err)
	}
	for _, want := range []string{"runqlat", "offcputime"} {
		if !strings.Contains(reason, want) {
			t.Errorf("refusal %q does not tell the model what it could ask for (%q)", reason, want)
		}
	}
}

func TestRegistry_OpsTraceDescriptionNamesEveryCatalogueScript(t *testing.T) {
	t.Parallel()

	definition, ok := registryDefinitions[OpsTraceToolName]
	if !ok {
		t.Fatal("ops_trace has no definition, so the model never learns it exists")
	}

	for _, script := range opsremote.TraceScripts() {
		if !strings.Contains(definition.Description, script.ID) {
			t.Errorf("the description does not name %q, so the model cannot choose it", script.ID)
		}
	}
	if !strings.Contains(definition.Description, "OPSAGI_SANDBOX_BPF") {
		t.Error("the description does not say what turns the tool on")
	}
}

func TestFormatLogs_CutTailReportsHowManyLinesWereDropped(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	for lines := 1; lines <= 400; lines++ {
		fmt.Fprintf(&stdout, "2026-11-23T04:21:%02dZ line %d the guard rejected the connection to 10.0.0.7\n", lines%60, lines)
	}

	formatted := formatLogs(
		opsremote.Target{ID: "lab", Host: "192.0.2.10", Port: 22},
		"容器 opslab-log-apache 日志",
		opsremote.Result{ExitCode: 0, Stdout: stdout.String()},
	)

	if want := "共 400 行"; !strings.Contains(formatted, want) {
		t.Errorf("result does not state the total line count (%q), so a partial sample reads as the whole log", want)
	}
	if !strings.Contains(formatted, "line 400 the guard rejected") {
		t.Error("the newest line was not kept")
	}
	if strings.Contains(formatted, "line 1 the guard rejected") {
		t.Error("the oldest line survived, so the cut is not a tail")
	}
	if len(formatted) > 12*1024 {
		t.Errorf("result is %d bytes, which overruns the size the tool promises the model", len(formatted))
	}
}

func TestOpsDiff_HandleDiff_TakesTheSampleIdsBaselineReturned(t *testing.T) {
	t.Parallel()

	tools := &opsTools{}
	before := opsremote.Fingerprint{Label: "before", Host: "lab", Load1: 22.4, CPUPercent: 88}
	after := opsremote.Fingerprint{Label: "after", Host: "lab", Load1: 0.9, CPUPercent: 12}
	beforeID := tools.samples.put(before)
	afterID := tools.samples.put(after)

	args, err := json.Marshal(OpsDiffArgs{Before: beforeID, After: afterID})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	rendered, err := tools.HandleDiff(t.Context(), OpsDiffToolName, args)
	if err != nil {
		t.Fatalf("the ids ops_baseline printed were refused: %v", err)
	}

	for _, want := range []string{"before", "after", "22.4", "0.9"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("diff %q does not show %q, so the two samples were not the ones the ids named", rendered, want)
		}
	}
}

func TestOpsDiff_HandleDiff_NamesTheSideWhoseSampleIsGone(t *testing.T) {
	t.Parallel()

	tools := &opsTools{}
	afterID := tools.samples.put(opsremote.Fingerprint{Label: "after", Host: "lab", Load1: 0.9})

	args, err := json.Marshal(OpsDiffArgs{Before: "base-99", After: afterID})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	_, err = tools.HandleDiff(t.Context(), OpsDiffToolName, args)
	if err == nil {
		t.Fatal("an id that names no sample was accepted")
	}
	for _, want := range []string{"before", "base-99", "只在采集它的那条 flow 里有效"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not tell the model %q, so it cannot tell a typo from a sample taken in another flow", err, want)
		}
	}
}

func TestOpsDiff_HandleDiff_StillTakesTheFingerprintItself(t *testing.T) {
	t.Parallel()

	tools := &opsTools{}
	args, err := json.Marshal(OpsDiffArgs{
		Before: `{"label":"before","host":"lab","load1":22.4}`,
		After:  `{"label":"after","host":"lab","load1":0.9}`,
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	rendered, err := tools.HandleDiff(t.Context(), OpsDiffToolName, args)
	if err != nil {
		t.Fatalf("a caller that pasted the fingerprint itself was refused: %v", err)
	}
	if !strings.Contains(rendered, "22.4") {
		t.Errorf("diff %q does not carry the pasted numbers", rendered)
	}
}

func TestContainers_FormatContainers_StatesTheMatchCountSoNobodyCountsRows(t *testing.T) {
	t.Parallel()

	all := []opsremote.Container{
		{Name: "seal-engraver-backend", State: "running", Image: "img/backend"},
		{Name: "seal-engraver-postgres", State: "exited", Image: "postgres:16"},
		{Name: "opslab-db", State: "running", Image: "postgres:16"},
	}
	target := opsremote.Target{ID: "lab", Host: "127.0.0.1", Port: 2222}

	filtered := formatContainers(target, all, filterContainers(all, "seal-engraver-"), nil, "seal-engraver-")
	if !strings.Contains(filtered, `名字里包含 "seal-engraver-" 的容器 2 个（这台主机上共 3 个）`) {
		t.Errorf("filtered listing = %q, want the count stated in the header", filtered)
	}
	if strings.Contains(filtered, "opslab-db") {
		t.Errorf("filtered listing = %q, want the container the filter excludes left out", filtered)
	}

	everything := formatContainers(target, all, all, nil, "")
	if !strings.Contains(everything, "容器 3 个") {
		t.Errorf("unfiltered listing = %q, want the total stated", everything)
	}
}

func TestContainers_FormatContainers_TellsAnEmptyMatchFromAHostWithoutDocker(t *testing.T) {
	t.Parallel()

	target := opsremote.Target{ID: "lab", Host: "127.0.0.1", Port: 2222}
	all := []opsremote.Container{{Name: "opslab-db", State: "running"}}

	empty := formatContainers(target, all, filterContainers(all, "nginx"), nil, "nginx")
	for _, want := range []string{`名字里包含 "nginx" 的容器 0 个`, "共 1 个", "不带 filter"} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty match = %q, want it to carry %q", empty, want)
		}
	}
	if strings.Contains(empty, "未安装 Docker") {
		t.Errorf("empty match = %q, want no claim that Docker is missing", empty)
	}

	bare := formatContainers(target, nil, nil, nil, "nginx")
	if !strings.Contains(bare, "没有检测到容器") {
		t.Errorf("host without containers = %q, want the Docker hint", bare)
	}
}

func TestContainers_FilterContainers_KeepsEveryContainerWithoutAFilter(t *testing.T) {
	t.Parallel()

	all := []opsremote.Container{{Name: "a"}, {Name: "b"}}
	if got := filterContainers(all, ""); len(got) != 2 {
		t.Errorf("filtered = %+v, want the whole listing back for an empty filter", got)
	}
	if got := filterContainers(all, "b"); len(got) != 1 || got[0].Name != "b" {
		t.Errorf("filtered = %+v, want only b", got)
	}
}

func TestOpsTools_HandleApply_RefusesAnUnknownActionBeforeItDials(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}

	_, err := tools.WithWrites(true).HandleApply(t.Context(), OpsApplyToolName, []byte(`{"action":"rm_rf"}`))

	if err == nil {
		t.Fatal("an action outside the catalogue was accepted")
	}
	for _, id := range opsremote.ActionIDs() {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error = %q, want it to name %s as available", err, id)
		}
	}
}

func TestOpsTools_HandleApply_RefusesAnArgumentCarryingAShellBeforeItDials(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}

	_, err := tools.WithWrites(true).HandleApply(t.Context(), OpsApplyToolName, []byte(`{"action":"container_restart","container":"api; reboot"}`))

	if err == nil {
		t.Fatal("a container name carrying a shell was accepted")
	}
	if !strings.Contains(err.Error(), "非法") {
		t.Errorf("error = %q, want the argument refusal", err)
	}
}

func TestOpsTools_HandleApply_AnswersAnEmptyActionWithTheCatalogue(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}

	_, err := tools.WithWrites(true).HandleApply(t.Context(), OpsApplyToolName, []byte(`{}`))

	if err == nil {
		t.Fatal("a call with no action was accepted")
	}
	for _, id := range opsremote.ActionIDs() {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error = %q, want it to name %s", err, id)
		}
	}
}

func TestRegistry_OpsApplyDescriptionNamesEveryCatalogueAction(t *testing.T) {
	definition, ok := registryDefinitions[OpsApplyToolName]
	if !ok {
		t.Fatal("ops_apply has no definition")
	}

	for _, id := range opsremote.ActionIDs() {
		if !strings.Contains(definition.Description, id) {
			t.Errorf("the description does not name %s, so the model cannot choose it", id)
		}
	}
}

func TestOpsTools_HandleApply_RefusesWhenTheFlowHasNoEnforcingLease(t *testing.T) {
	tools := &opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}

	_, err := tools.HandleApply(t.Context(), OpsApplyToolName, []byte(`{"action":"container_restart","container":"api-1"}`))

	if err == nil {
		t.Fatal("a repair was accepted without a lease to judge it")
	}
	if !strings.Contains(err.Error(), "enforce") {
		t.Errorf("error = %q, want it to name the mode a write needs", err)
	}
}

func TestOpsTools_WithWrites_IsTheSwitchTheHandlerObeys(t *testing.T) {
	tools := (&opsTools{targets: map[string]opsremote.Target{"lab": {ID: "lab", Host: "192.0.2.10"}}}).WithWrites(true)

	// An unknown action is refused by the catalogue, which is what proves the
	// call got past the mode check without needing a host to dial.
	_, err := tools.HandleApply(t.Context(), OpsApplyToolName, []byte(`{"target":"lab","action":"rm_rf"}`))

	if err == nil {
		t.Fatal("an action outside the catalogue was accepted")
	}
	if strings.Contains(err.Error(), "enforce") {
		t.Errorf("error = %q, want the mode check to have passed", err)
	}
	if !strings.Contains(err.Error(), "rm_rf") {
		t.Errorf("error = %q, want the catalogue refusal", err)
	}
}
