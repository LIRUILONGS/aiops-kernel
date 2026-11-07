package opsremote

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SandboxImageDefault is what the probe box runs unless the operator names
// another image. A stock Debian is enough for the box to be useful: it brings a
// shell and the /proc the box is pointed at, and the tools an operator adds to
// its own image are the tools the agent gets.
const SandboxImageDefault = "debian:latest"

// SandboxMode is which shape the box takes. Both shapes leave the host as
// read-only as the mounts make it; they differ in whether the box can write its
// own layer and reach the network, which is what installing a tool needs.
type SandboxMode string

const (
	// SandboxModeProbe reads and nothing else: no network, read-only root.
	SandboxModeProbe SandboxMode = "probe"
	// SandboxModeWorkshop is the same box with a writable layer and egress, so the
	// agent installs the tool it decided it needs instead of the operator
	// pre-deciding a toolbox image. The host stays read-only either way.
	SandboxModeWorkshop SandboxMode = "workshop"
)

// sandboxFlags are the guarantee. Read-only on the host is a property of the
// mounts and the dropped capabilities, not of the script: /proc and /sys are
// bound read-only, every capability is gone, and the host pid namespace is not
// joined, so a pid that is visible under /host/proc cannot be signalled.
const sandboxFlags = "docker run --rm -i" +
	" --cap-drop=ALL --security-opt=no-new-privileges" +
	" --pids-limit=256 --cpus=1" +
	" -v /proc:/host/proc:ro -v /sys:/host/sys:ro"

// sandboxProbeFlags make the box unable to write even its own layer.
const sandboxProbeFlags = " --network=none --read-only --memory=256m"

// sandboxWorkshopFlags give the box a writable layer and egress. The memory
// ceiling is higher because installing packages is not a read, and setuid/setgid
// come back because apt drops privileges to _apt before fetching and dies with
// "setgroups 0 failed" without them. Neither capability reaches the host: the
// mounts stay read-only and the host pid namespace is still not joined.
const sandboxWorkshopFlags = " --memory=512m --cap-add=SETUID --cap-add=SETGID"

// sandboxScriptPath is where the box reads the script from, via -i and sh -s.
const sandboxScriptPath = "sh -s"

var sandboxImagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*$`)

// SandboxCommand is the remote command line for one run: fixed flags for the
// mode, a validated image name, and nothing the agent wrote. The script travels
// on stdin, so no amount of quoting in it can reach the shell that starts docker.
func SandboxCommand(image string, mode SandboxMode) (string, error) {
	return sandboxCommandNamed(image, "", mode)
}

// sandboxCommandNamed adds a name, which is how a run that outlived its timeout
// is found again and removed: killing the ssh session leaves the container
// running whatever the script started.
func sandboxCommandNamed(image, name string, mode SandboxMode) (string, error) {
	if image == "" {
		image = SandboxImageDefault
	}

	var shape string
	switch mode {
	case "", SandboxModeProbe:
		shape = sandboxProbeFlags
	case SandboxModeWorkshop:
		shape = sandboxWorkshopFlags
	default:
		return "", fmt.Errorf("未知的采集容器形状 %q：只能是 %s 或 %s", mode, SandboxModeProbe, SandboxModeWorkshop)
	}

	named := ""
	if name != "" {
		named = " --name " + name
	}

	return sandboxFlags + shape + named + " " + image + " " + sandboxScriptPath, nil
}

// sandboxName is the name a probe container gets. Random rather than derived from
// the call, so two flows probing one host cannot collide on it.
func sandboxName() string {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return ""
	}

	return "opsagi-probe-" + hex.EncodeToString(suffix)
}

// ValidateSandboxImage keeps the image name from carrying anything but a name.
// The allowlist is the operator's: an image the box has never run may pull
// layers, and pulling is a write on the target.
func ValidateSandboxImage(image string, allowed []string) (string, error) {
	if image == "" {
		image = SandboxImageDefault
	}

	if !sandboxImagePattern.MatchString(image) {
		return "", fmt.Errorf("镜像名 %q 含不允许的字符", image)
	}

	for _, candidate := range allowed {
		if candidate == image {
			return image, nil
		}
	}

	return "", fmt.Errorf("镜像 %q 不在允许列表（%s）", image, strings.Join(allowed, ", "))
}

// RunSandbox runs one script inside the box on the target. A run that outlives
// its deadline is removed rather than left sampling: a script that watches
// forever would keep costing the host it is watching.
func (c *Client) RunSandbox(ctx context.Context, image, script string, mode SandboxMode) (Result, error) {
	command, err := sandboxCommandNamed(image, "", mode)
	if err != nil {
		return Result{}, err
	}

	name := sandboxName()
	if name != "" {
		command, err = sandboxCommandNamed(image, name, mode)
		if err != nil {
			return Result{}, err
		}
	}

	res, err := c.RunStdin(ctx, command, script)
	if err != nil && ctx.Err() != nil && name != "" {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_, _ = c.Run(cleanup, "docker kill "+name+" >/dev/null 2>&1; docker rm -f "+name+" >/dev/null 2>&1")
	}

	return res, err
}

// SandboxProbe is the first call of every host-resource task: what the box can
// see of the host, in one round trip. Tooling on the host itself is a question
// for the fixed catalogue (file_stat on an absolute path), because nothing on
// the host is mounted except /proc and /sys.
const SandboxProbe = `echo "== kernel"; cat /host/proc/sys/kernel/osrelease
echo "== init"; cat /host/proc/1/comm
echo "== pressure"; for f in cpu io memory; do printf "%s: " "$f"; head -1 /host/proc/pressure/$f 2>/dev/null || echo "unavailable"; done
echo "== perf_event_paranoid"; cat /host/proc/sys/kernel/perf_event_paranoid 2>/dev/null || echo "unavailable"
echo "== procs"; ls /host/proc | grep -c "^[0-9]"
echo "== load"; cat /host/proc/loadavg`

