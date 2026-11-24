package opsremote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsRemote_RenderAction_RendersEveryCatalogueEntry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       string
		args     map[string]string
		command  string
		instance string
	}{
		{
			name:     "unit restart",
			id:       "unit_restart",
			args:     map[string]string{"Unit": "nginx.service"},
			command:  "systemctl restart nginx.service && systemctl is-active nginx.service && systemctl show -p ActiveEnterTimestamp --value nginx.service",
			instance: "unit_restart nginx.service",
		},
		{
			name:     "container restart",
			id:       "container_restart",
			args:     map[string]string{"Container": "api-1"},
			command:  "docker restart api-1 >/dev/null && docker inspect --format '{{.State.Status}} {{.State.StartedAt}} {{.RestartCount}}' api-1",
			instance: "container_restart api-1",
		},
		{
			name:     "config restore",
			id:       "config_restore",
			args:     map[string]string{"Backup": "/var/lib/opsagi/backups/nginx.conf.bak", "Path": "/etc/nginx/nginx.conf"},
			command:  "install -m 0644 /var/lib/opsagi/backups/nginx.conf.bak /etc/nginx/nginx.conf && stat -c '%n %s %y' /etc/nginx/nginx.conf",
			instance: "config_restore /etc/nginx/nginx.conf",
		},
		{
			name:     "disk cleanup defaults to a week",
			id:       "disk_cleanup",
			args:     map[string]string{"Path": "/var/log/nginx/"},
			command:  "find /var/log/nginx/ -xdev -type f -atime +7 -print -delete | wc -l",
			instance: "disk_cleanup /var/log/nginx/",
		},
		{
			name:     "k8s rollout restart",
			id:       "k8s_rollout_restart",
			args:     map[string]string{"Namespace": "default", "Kind": "deployment", "Name": "api"},
			command:  "kubectl -n default rollout restart deployment/api && kubectl -n default rollout status deployment/api --timeout=60s",
			instance: "k8s_rollout_restart api",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			command, instance, err := RenderAction(tc.id, tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.command, command)
			assert.Equal(t, tc.instance, instance)
		})
	}
}

func TestOpsRemote_RenderAction_RefusesAnActionThatIsNotInTheCatalogue(t *testing.T) {
	t.Parallel()

	_, _, err := RenderAction("rm_rf", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rm_rf")
	for _, id := range ActionIDs() {
		assert.Contains(t, err.Error(), id, "the refusal does not name %s as available", id)
	}
}

func TestOpsRemote_RenderAction_RefusesAPathThatLeavesItsDirectory(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   string
		args map[string]string
		why  string
	}{
		{
			name: "a restore from outside the backup directory",
			id:   "config_restore",
			args: map[string]string{"Backup": "/root/.ssh/id_ed25519", "Path": "/etc/nginx/nginx.conf"},
			why:  "/var/lib/opsagi/backups/",
		},
		{
			name: "a restore over a file the template may not touch",
			id:   "config_restore",
			args: map[string]string{"Backup": "/var/lib/opsagi/backups/bak", "Path": "/root/.ssh/authorized_keys"},
			why:  "/etc/",
		},
		{
			name: "a cleanup aimed at the ssh keys",
			id:   "disk_cleanup",
			args: map[string]string{"Path": "/etc/ssh/"},
			why:  "/var/log/",
		},
		{
			name: "a prefix that is walked out of",
			id:   "disk_cleanup",
			args: map[string]string{"Path": "/var/log/../../etc/ssh/"},
			why:  "..",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := RenderAction(tc.id, tc.args)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.why)
		})
	}
}

func TestOpsRemote_RenderAction_RefusesAKubernetesKindTheHostWouldNotKnow(t *testing.T) {
	t.Parallel()

	_, _, err := RenderAction("k8s_rollout_restart", map[string]string{"Namespace": "default", "Kind": "pod", "Name": "api"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deployment")
}

func TestOpsRemote_RenderAction_RefusesAValueCarryingAShell(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"api-1; reboot", "api-1 && curl x", "$(reboot)", "api 1"} {
		_, _, err := RenderAction("container_restart", map[string]string{"Container": value})

		require.Error(t, err, "value %q was accepted", value)
		assert.True(t, strings.Contains(err.Error(), "非法"), err.Error())
	}
}

func TestOpsRemote_ActionCall_NamesAWriteWithoutItsIdWhenTheInstanceIsMissing(t *testing.T) {
	t.Parallel()

	// A write whose object is not named is still a distinct call, and an approval
	// for it must not be mistaken for an approval of some instance of it.
	assert.Equal(t, "container_restart", ActionCall("container_restart", nil))
	assert.Equal(t, "container_restart api-1", ActionCall("container_restart", map[string]string{"Container": "api-1"}))
	assert.Equal(t, "", ActionCall("not_a_write", nil))
}
