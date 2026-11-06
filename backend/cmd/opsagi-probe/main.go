// Command opsagi-probe collects read-only diagnostics from a remote host and
// prints them as JSON. It is the verification harness for the ops tools: the
// same code runs inside the agent, so anything it reports the agent can see.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"opsagi/pkg/opsremote"
)

func main() {
	var (
		host     = flag.String("host", os.Getenv("OPSAGI_SSH_HOST"), "target host")
		port     = flag.Int("port", envInt("OPSAGI_SSH_PORT", 22), "target ssh port")
		user     = flag.String("user", envDefault("OPSAGI_SSH_USER", "root"), "ssh user")
		password = flag.String("password", os.Getenv("OPSAGI_SSH_PASSWORD"), "ssh password")
		keyFile  = flag.String("key", os.Getenv("OPSAGI_SSH_KEY"), "ssh private key file")
		logs     = flag.String("logs", "", "also dump the tail of this container's logs")
		asJSON   = flag.Bool("json", false, "print the raw JSON report")
		timeout  = flag.Duration("timeout", 60*time.Second, "overall deadline")
	)
	flag.Parse()

	if *host == "" {
		fatal("target host is required (-host or OPSAGI_SSH_HOST)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	target := opsremote.Target{
		ID:       "cli",
		Host:     *host,
		Port:     *port,
		User:     *user,
		Password: *password,
		KeyFile:  *keyFile,
	}

	client, err := opsremote.Dial(ctx, target)
	if err != nil {
		fatal("connect: " + err.Error())
	}
	defer func() { _ = client.Close() }()

	report := client.Collect(ctx)

	var logResult *opsremote.Result
	if *logs != "" {
		res, logErr := client.ContainerLogs(ctx, *logs, 120)
		if logErr != nil {
			report.Errors = append(report.Errors, "logs: "+logErr.Error())
		} else {
			logResult = &res
		}
	}

	if *asJSON {
		payload := struct {
			opsremote.Report
			Logs *opsremote.Result `json:"logs,omitempty"`
		}{Report: report, Logs: logResult}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fatal("encode: " + err.Error())
		}
		return
	}

	printReport(report, logResult)
}

func printReport(report opsremote.Report, logs *opsremote.Result) {
	snap := report.System

	fmt.Printf("target   %s@%s:%d\n", report.Target.User, report.Target.Host, report.Target.Port)
	fmt.Printf("host     %s\n", snap.Hostname)
	fmt.Printf("uptime   %.1f h\n", snap.UptimeSec/3600)
	fmt.Printf("load     %.2f %.2f %.2f on %d cores\n", snap.Load1, snap.Load5, snap.Load15, snap.CPUCores)
	fmt.Printf("cpu      %.1f%%\n", snap.CPUUsage)
	fmt.Printf("memory   %d/%d MB used (%.0f%%), swap %d/%d MB\n",
		snap.Memory.TotalMB-snap.Memory.AvailableMB, snap.Memory.TotalMB,
		snap.Memory.PercentUsed(), snap.Memory.SwapUsedMB, snap.Memory.SwapTotalMB)

	fmt.Println("\nfilesystems")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  mount\tused\tavail\tuse%")
	for _, fs := range snap.Filesystems {
		fmt.Fprintf(w, "  %s\t%dMB\t%dMB\t%.0f%%\n", fs.MountPoint, fs.UsedKB/1024, fs.AvailKB/1024, fs.UsePercent)
	}
	_ = w.Flush()

	fmt.Println("\ntop processes")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  pid\tcpu%\tmem%\trss\tcommand")
	for _, p := range snap.TopProcesses {
		fmt.Fprintf(w, "  %d\t%.1f\t%.1f\t%dMB\t%s\n", p.PID, p.CPU, p.Memory, p.RSSKB/1024, p.Command)
	}
	_ = w.Flush()

	fmt.Println("\ncontainers")
	if len(report.Containers) == 0 {
		fmt.Println("  (none)")
	}
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  name\tstate\trestarts\texit\timage")
	for _, c := range report.Containers {
		flagNote := ""
		if c.RestartCount > 3 {
			flagNote = "  <-- crash loop"
		}
		fmt.Fprintf(w, "  %s\t%s\t%d\t%d\t%s%s\n", c.Name, c.State, c.RestartCount, c.ExitCode, c.Image, flagNote)
	}
	_ = w.Flush()

	if len(report.FailedUnits) > 0 {
		fmt.Println("\nfailed units")
		for _, u := range report.FailedUnits {
			fmt.Println("  " + u)
		}
	}

	if logs != nil {
		fmt.Printf("\nlogs (%s)\n", logs.Command)
		fmt.Println(strings_TrimTrailing(logs.Stdout, 4000))
	}

	if len(report.Errors) > 0 {
		fmt.Println("\ncollection errors")
		for _, e := range report.Errors {
			fmt.Println("  " + e)
		}
	}
}

// strings_TrimTrailing keeps the last n bytes of s so a long log dump does not
// bury the report. Named to avoid importing strings only for this.
func strings_TrimTrailing(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "... (truncated, last " + fmt.Sprint(n) + " bytes)\n" + s[len(s)-n:]
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "opsagi-probe: "+msg)
	os.Exit(1)
}
