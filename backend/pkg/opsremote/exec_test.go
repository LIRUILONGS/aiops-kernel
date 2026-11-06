package opsremote

import (
	"strings"
	"testing"
)

func TestExec_Render_RefusesAnythingThatCouldLeaveTheCatalogue(t *testing.T) {
	tests := []struct {
		name string
		id   string
		args map[string]string
		want string
	}{
		{
			name: "an unknown command is answered with the catalogue",
			id:   "run_whatever",
			want: "未知的只读命令",
		},
		{
			name: "a container name carrying a shell is refused",
			id:   "docker_inspect",
			args: map[string]string{"Container": "nginx; rm -rf /"},
			want: "非法",
		},
		{
			name: "a container name carrying a substitution is refused",
			id:   "docker_inspect",
			args: map[string]string{"Container": "$(id)"},
			want: "非法",
		},
		{
			name: "a name with a newline is refused",
			id:   "unit_status",
			args: map[string]string{"Unit": "sshd\nwhoami"},
			want: "非法",
		},
		{
			name: "a relative path is refused",
			id:   "file_stat",
			args: map[string]string{"Path": "etc/nginx/nginx.conf"},
			want: "非法",
		},
		{
			name: "a path with a redirection is refused",
			id:   "file_stat",
			args: map[string]string{"Path": "/etc/nginx/nginx.conf > /etc/passwd"},
			want: "非法",
		},
		{
			name: "an image reference carrying a command substitution is refused",
			id:   "image_history",
			args: map[string]string{"Image": "nginx$(id)"},
			want: "非法",
		},
		{
			name: "an image reference starting with a dash is refused",
			id:   "image_history",
			args: map[string]string{"Image": "--help"},
			want: "非法",
		},
		{
			name: "a missing image reference is refused",
			id:   "image_history",
			want: "缺少参数",
		},
		{
			name: "a missing name is refused",
			id:   "unit_logs",
			want: "缺少参数",
		},
		{
			name: "a line count outside the range is refused",
			id:   "unit_logs",
			args: map[string]string{"Unit": "sshd", "Lines": "100000"},
			want: "超出范围",
		},
		{
			name: "a non-numeric line count is refused",
			id:   "kernel_log",
			args: map[string]string{"Lines": "many"},
			want: "需要一个整数",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.id, tc.args)
			if err == nil {
				t.Fatalf("rendered %q, want an error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestExec_Render_BuildsTheCommandFromTheTemplateAndTheValidatedValue(t *testing.T) {
	tests := []struct {
		name string
		id   string
		args map[string]string
		want string
	}{
		{
			name: "a command without parameters renders its template unchanged",
			id:   "listen_ports",
			want: "ss -lntup 2>/dev/null",
		},
		{
			name: "a name lands where the template put it",
			id:   "docker_inspect",
			args: map[string]string{"Container": "opslab-broken"},
			want: "docker inspect opslab-broken --format",
		},
		{
			name: "a name given with its leading slash is normalised",
			id:   "unit_status",
			args: map[string]string{"Unit": "/sshd.service"},
			want: "systemctl status sshd.service --no-pager -l 2>&1",
		},
		{
			name: "an omitted count takes the default",
			id:   "top_cpu",
			want: "head -n 15",
		},
		{
			name: "a count inside the range is used",
			id:   "kernel_log",
			args: map[string]string{"Lines": "250"},
			want: "tail -n 250",
		},
		{
			name: "both parameters of a two-parameter command are rendered",
			id:   "unit_logs",
			args: map[string]string{"Unit": "docker.service", "Lines": "50"},
			want: "journalctl -u docker.service -n 50 --no-pager -o short-iso 2>/dev/null",
		},
		{
			name: "a path is rendered verbatim",
			id:   "file_stat",
			args: map[string]string{"Path": "/etc/nginx/nginx.conf"},
			want: "stat /etc/nginx/nginx.conf 2>&1",
		},
		{
			name: "an image reference keeps its registry, tag and digest",
			id:   "image_history",
			args: map[string]string{"Image": "ghcr.io/vx/opsagi:1.2.3@sha256:abc123"},
			want: "ghcr.io/vx/opsagi:1.2.3@sha256:abc123",
		},
		{
			name: "the package history takes its default when no count is given",
			id:   "package_history",
			want: "tail -n 40",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.id, tc.args)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("command = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestExec_Catalogue_DescribesEveryEntryAndKeepsTheOrder(t *testing.T) {
	commands := Commands()
	if len(commands) != len(catalogOrder) {
		t.Fatalf("got %d commands, want %d", len(commands), len(catalogOrder))
	}

	seen := make(map[string]bool, len(commands))
	for i, command := range commands {
		if command.ID != catalogOrder[i] {
			t.Errorf("command %d = %q, want %q", i, command.ID, catalogOrder[i])
		}
		if seen[command.ID] {
			t.Errorf("duplicate id %q", command.ID)
		}
		seen[command.ID] = true

		if command.Description == "" {
			t.Errorf("command %q has no description", command.ID)
		}
		if command.Template == "" {
			t.Errorf("command %q has no template", command.ID)
		}
		for _, param := range command.Params {
			if !strings.Contains(command.Template, "%"+param.Template+"%") {
				t.Errorf("command %q declares %s but its template never uses it", command.ID, param.Template)
			}
		}
	}
}
