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

// sandboxPreamble is the whole guarantee. The agent writes a script and the
// script runs here, so read-only is a property of the box rather than a promise
// about the command text: nothing writable is mounted, there is no network,
// every capability is dropped, and a signal cannot reach a host process because
// the host pid namespace is not joined. /proc is bound read-only, which is
// enough to read a process's state and its per-pid view of the network stack
// (/host/proc/1/net/tcp), and not enough to signal it.
const sandboxPreamble = "docker run --rm -i" +
	" --network=none --read-only" +
	" --cap-drop=ALL --security-opt=no-new-privileges" +
	" --pids-limit=256 --memory=256m --cpus=1" +
	" -v /proc:/host/proc:ro -v /sys:/host/sys:ro"

// sandboxScriptPath is where the box reads the script from, via -i and sh -s.
const sandboxScriptPath = "sh -s"

var sandboxImagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*$`)

// SandboxCommand is the remote command line for a probe run: fixed flags, a
// validated image name, and nothing the agent wrote. The script travels on
// stdin, so no amount of quoting in it can reach the shell that starts docker.
func SandboxCommand(image string) string {
	return SandboxCommandNamed(image, "")
}

// SandboxCommandNamed is SandboxCommand with a name, which is how a probe that
// outlived its timeout is found again and removed: killing the ssh session leaves
// the container running its loop on the target.
func SandboxCommandNamed(image, name string) string {
	if image == "" {
		image = SandboxImageDefault
	}

	named := ""
	if name != "" {
		named = " --name " + name
	}

	return sandboxPreamble + named + " " + image + " " + sandboxScriptPath
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

// RunSandbox runs one script inside the probe box on the target. A probe that
// runs past its deadline is removed rather than left sampling: a script that
// watches forever would keep costing the host it is watching.
func (c *Client) RunSandbox(ctx context.Context, image, script string) (Result, error) {
	name := sandboxName()

	res, err := c.RunStdin(ctx, SandboxCommandNamed(image, name), script)
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

