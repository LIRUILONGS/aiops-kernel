package opsremote

import (
	"strings"
	"testing"
)

// A runqlat dump as bpftrace prints it: one hist() block per command name, in
// microseconds, with the power-of-two ranges hist() uses.
const runqlatDump = `Attaching 2 probes...

@us[java]:
[1K, 2K)       12 |@@@@@@@@                                            |
[2K, 4K)       30 |@@@@@@@@@@@@@@@@@@@@                                |
[4K, 8K)        5 |@@@                                                 |

@us[python3]:
[8K, 16K)       3 |@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@|
`

func TestTrace_ParseTraceHistograms_ReadsEveryBlockKeyedByCommand(t *testing.T) {
	t.Parallel()

	hists := ParseTraceHistograms(runqlatDump)
	if len(hists) != 2 {
		t.Fatalf("histograms = %d, want the two blocks the dump carries", len(hists))
	}

	if hists[0].Key != "java" || hists[0].Total != 47 {
		t.Errorf("first = %s/%d, want java/47", hists[0].Key, hists[0].Total)
	}

	if hists[1].Key != "python3" || hists[1].Total != 3 {
		t.Errorf("second = %s/%d, want python3/3", hists[1].Key, hists[1].Total)
	}

	if got := hists[0].Buckets[0]; got.Low != 1000 || got.High != 2000 || got.Count != 12 {
		t.Errorf("first bucket = %+v, want [1000, 2000) ×12", got)
	}
}

func TestTrace_ParseTraceHistograms_IgnoresOutputThatIsNotAHistogram(t *testing.T) {
	t.Parallel()

	counting := `@[bash, /usr/bin/ls]: 14

@[nginx, /usr/sbin/nginx]: 2
`
	if hists := ParseTraceHistograms(counting); len(hists) != 0 {
		t.Errorf("histograms = %+v, want none for a count() program", hists)
	}

	if summary := SummarizeTrace(counting); summary != "" {
		t.Errorf("summary = %q, want nothing invented for a listing", summary)
	}
}

func TestTrace_Histogram_QuantilePicksTheBucketTheObservationFallsIn(t *testing.T) {
	t.Parallel()

	hist := TraceHistogram{
		Key:   "java",
		Total: 50,
		Buckets: []TraceBucket{
			{Low: 1000, High: 2000, Count: 12},
			{Low: 2000, High: 4000, Count: 30},
			{Low: 4000, High: 8000, Count: 5},
			{Low: 8000, High: 16000, Count: 3},
		},
	}

	for _, want := range []struct {
		p     float64
		micro float64
	}{
		{0.50, 3000},
		{0.90, 6000},
		{0.99, 12000},
	} {
		if got := hist.Quantile(want.p); got != want.micro {
			t.Errorf("p%.0f = %.0fµs, want %.0fµs", want.p*100, got, want.micro)
		}
	}

	if got := (TraceHistogram{}).Quantile(0.99); got != 0 {
		t.Errorf("empty histogram quantile = %v, want 0", got)
	}
}

func TestTrace_SummarizeTrace_NamesTheNumbersAReportQuotes(t *testing.T) {
	t.Parallel()

	summary := SummarizeTrace(runqlatDump)
	if summary == "" {
		t.Fatal("a histogram produced no summary")
	}

	lines := strings.Split(summary, "\n")
	if len(lines) != 2 {
		t.Fatalf("summary = %q, want a totals line and a slowest-process line", summary)
	}

	for _, want := range []string{"共 50 次等待", "p50 3.00ms", "p90 6.00ms", "p99 12.00ms"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("totals = %q, want it to carry %q", lines[0], want)
		}
	}

	for _, want := range []string{"java", "占 79%", "p99 6.00ms"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("slowest = %q, want it to carry %q", lines[1], want)
		}
	}
}

func TestTrace_SummarizeTrace_ReadsMicrosecondAndSecondBuckets(t *testing.T) {
	t.Parallel()

	dump := `@us[worker]:
[1, 2)         4 |@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@|

@us[stuck]:
[2M, 4M)       2 |@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@|
`

	summary := SummarizeTrace(dump)
	for _, want := range []string{"共 6 次等待", "p50 1.5µs", "p99 3.00s", "stuck"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary = %q, want it to carry %q", summary, want)
		}
	}
}

func TestTrace_SummarizeTrace_MergesRangesMissingFromOneSubject(t *testing.T) {
	t.Parallel()

	dump := `@us[a]:
[1K, 2K)       1 |@@@|

@us[b]:
[2K, 4K)       3 |@@@@@@@@@|
`

	summary := SummarizeTrace(dump)
	if !strings.Contains(summary, "共 4 次等待") {
		t.Fatalf("summary = %q, want the four waits from both subjects", summary)
	}
	if !strings.Contains(summary, "p50 3.00ms") {
		t.Errorf("summary = %q, want the second observation in the 3ms bucket", summary)
	}
}
