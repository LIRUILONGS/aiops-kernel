package opsremote

import (
	"fmt"
	"sort"
	"strings"
)

// Action is one repair an agent may apply by name. The template is a constant of
// this package and every parameter is validated before it reaches the command
// line, so a repair is a choice from a list — never a command the model
// composed. That is the whole difference between this catalogue and a shell.
type Action struct {
	ID          string
	Description string
	Template    string
	Params      []Param
	// Instance names the parameter that says what is being touched. An approval
	// quotes the result, so an operator approves "restart api-1" rather than "a
	// write", and the grant that follows covers that one object only.
	Instance string
}

var (
	applyOrder = []string{
		"unit_restart",
		"container_restart",
		"config_restore",
		"disk_cleanup",
		"k8s_rollout_restart",
	}
	applyCatalogue = map[string]Action{
		"unit_restart": {
			ID:          "unit_restart",
			Description: "重启一个 systemd 单元",
			Template: "systemctl restart %Unit% && systemctl is-active %Unit%" +
				" && systemctl show -p ActiveEnterTimestamp --value %Unit%",
			Params:   []Param{{Template: "Unit", Kind: ParamName}},
			Instance: "Unit",
		},
		"container_restart": {
			ID:          "container_restart",
			Description: "重启一个容器",
			Template: "docker restart %Container% >/dev/null" +
				" && docker inspect --format '{{.State.Status}} {{.State.StartedAt}} {{.RestartCount}}' %Container%",
			Params:   []Param{{Template: "Container", Kind: ParamName}},
			Instance: "Container",
		},
		"config_restore": {
			ID:          "config_restore",
			Description: "用备份目录里的一份快照覆盖配置文件",
			Template:    "install -m 0644 %Backup% %Path% && stat -c '%%n %%s %%y' %Path%",
			Params: []Param{
				{Template: "Backup", Kind: ParamPath, Prefixes: []string{"/var/lib/opsagi/backups/"}},
				{Template: "Path", Kind: ParamPath, Prefixes: []string{"/etc/"}},
			},
			Instance: "Path",
		},
		"disk_cleanup": {
			ID:          "disk_cleanup",
			Description: "删掉日志或临时目录里 N 天没被访问过的文件",
			Template:    "find %Path% -xdev -type f -atime +%Days% -print -delete | wc -l",
			Params: []Param{
				{Template: "Path", Kind: ParamPath, Prefixes: []string{"/var/log/", "/var/cache/", "/var/tmp/", "/tmp/"}},
				{Template: "Days", Kind: ParamCount, Min: 1, Max: 90, Default: 7},
			},
			Instance: "Path",
		},
		"k8s_rollout_restart": {
			ID:          "k8s_rollout_restart",
			Description: "重启一个 Kubernetes 工作负载并等它滚动完成",
			Template: "kubectl -n %Namespace% rollout restart %Kind%/%Name%" +
				" && kubectl -n %Namespace% rollout status %Kind%/%Name% --timeout=60s",
			Params: []Param{
				{Template: "Namespace", Kind: ParamName},
				{Template: "Kind", Kind: ParamOneOf, Options: []string{"deployment", "statefulset", "daemonset"}},
				{Template: "Name", Kind: ParamName},
			},
			Instance: "Name",
		},
	}
)

// Actions lists the write catalogue in a stable order.
func Actions() []Action {
	out := make([]Action, 0, len(applyOrder))
	for _, id := range applyOrder {
		out = append(out, applyCatalogue[id])
	}

	return out
}

// LookupAction returns the catalogue entry with this id.
func LookupAction(id string) (Action, bool) {
	action, ok := applyCatalogue[id]

	return action, ok
}

// ActionIDs lists the ids a caller may pass, sorted for a deterministic message.
func ActionIDs() []string {
	ids := make([]string, 0, len(applyCatalogue))
	for id := range applyCatalogue {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}

// RenderAction turns a catalogue id plus arguments into the command line to run
// and the name an approval quotes for it. The instance is what the lease and the
// approval compare: widening an approval from one container to its neighbours
// would be a different call, and the lease would have to judge it again.
func RenderAction(id string, args map[string]string) (string, string, error) {
	action, ok := applyCatalogue[id]
	if !ok {
		return "", "", fmt.Errorf("未知的写操作 %q，可用操作：%s", id, strings.Join(ActionIDs(), ", "))
	}

	command, err := renderTemplate(action.ID, action.Template, action.Params, args)
	if err != nil {
		return "", "", err
	}

	return command, ActionCall(id, args), nil
}

// ActionCall names a write the way a lease and an approval name it: the id plus
// the object it touches. A write whose instance parameter is empty is named by
// its id alone, which is strictly narrower than any instance of it.
func ActionCall(id string, args map[string]string) string {
	action, ok := applyCatalogue[id]
	if !ok {
		return ""
	}

	instance := strings.TrimSpace(args[action.Instance])
	if instance == "" {
		return id
	}

	return id + " " + instance
}

func renderTemplate(id, template string, params []Param, args map[string]string) (string, error) {
	rendered := template
	for _, param := range params {
		raw := strings.TrimSpace(args[param.Template])
		placeholder := "%" + param.Template + "%"

		value, err := renderParam(param, raw)
		if err != nil {
			return "", fmt.Errorf("%s：%w", id, err)
		}
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}

	rendered = strings.ReplaceAll(rendered, "%%", escapedPercent)

	if idx := strings.Index(rendered, "%"); idx >= 0 {
		return "", fmt.Errorf("命令模板 %q 还留着未替换的占位符", id)
	}

	return strings.ReplaceAll(rendered, escapedPercent, "%"), nil
}
