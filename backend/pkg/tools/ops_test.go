package tools

import (
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
