package tools

import (
	"strings"
	"testing"

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
