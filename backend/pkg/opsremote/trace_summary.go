package opsremote

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TraceBucket is one row of a bpftrace histogram, in microseconds. bpftrace
// prints power-of-two ranges, so a quantile can only name the bucket it fell
// into; the midpoint of that bucket is the estimate this file reports.
type TraceBucket struct {
	Low   float64
	High  float64
	Count int
}

// TraceHistogram is one traced subject's histogram: bpftrace keys hist() by
// whatever the program put inside the brackets, which is the command name for
// every script in the catalogue.
type TraceHistogram struct {
	Key     string
	Buckets []TraceBucket
	Total   int
}

var (
	histogramHeader = regexp.MustCompile(`^@\w+(?:\[(.*)\])?:\s*$`)
	histogramBucket = regexp.MustCompile(`^\[\s*([0-9.]+)([KMG]?)\s*,\s*([0-9.]+)([KMG]?)\s*\)\s+(\d+)`)
)

// ParseTraceHistograms reads the hist() blocks out of a bpftrace run. Anything
// that is not a histogram is ignored, so a program that prints counts or sums
// yields no histograms rather than an error.
func ParseTraceHistograms(output string) []TraceHistogram {
	var (
		hists []TraceHistogram
		cur   *TraceHistogram
	)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if m := histogramHeader.FindStringSubmatch(line); m != nil {
			hists = append(hists, TraceHistogram{Key: m[1]})
			cur = &hists[len(hists)-1]

			continue
		}

		m := histogramBucket.FindStringSubmatch(line)
		if m == nil || cur == nil {
			continue
		}

		low, errLow := strconv.ParseFloat(m[1], 64)
		high, errHigh := strconv.ParseFloat(m[3], 64)
		count, errCount := strconv.Atoi(m[5])
		if errLow != nil || errHigh != nil || errCount != nil {
			continue
		}

		cur.Buckets = append(cur.Buckets, TraceBucket{
			Low:   low * bucketScale(m[2]),
			High:  high * bucketScale(m[4]),
			Count: count,
		})
		cur.Total += count
	}

	kept := hists[:0]
	for _, hist := range hists {
		if hist.Total > 0 {
			kept = append(kept, hist)
		}
	}

	return kept
}

func bucketScale(suffix string) float64 {
	switch suffix {
	case "K":
		return 1e3
	case "M":
		return 1e6
	case "G":
		return 1e9
	}

	return 1
}

// Quantile returns the midpoint of the bucket the p-th observation falls into,
// in microseconds. A histogram cannot answer more precisely than that.
func (h TraceHistogram) Quantile(p float64) float64 {
	if h.Total == 0 {
		return 0
	}

	threshold := p * float64(h.Total)

	seen := 0
	for _, bucket := range h.Buckets {
		seen += bucket.Count
		if float64(seen) >= threshold {
			return (bucket.Low + bucket.High) / 2
		}
	}

	last := h.Buckets[len(h.Buckets)-1]

	return (last.Low + last.High) / 2
}

// Mean returns the count-weighted midpoint of the histogram, in microseconds.
func (h TraceHistogram) Mean() float64 {
	if h.Total == 0 {
		return 0
	}

	sum := 0.0
	for _, bucket := range h.Buckets {
		sum += float64(bucket.Count) * (bucket.Low + bucket.High) / 2
	}

	return sum / float64(h.Total)
}

// SummarizeTrace condenses a histogram trace into the numbers a report quotes:
// how many waits were seen, how long the p50/p90/p99 wait was, and which process
// waited the most. It returns "" for a program whose output carries no
// histogram, because a summary line invented for an execsnoop listing would be
// worse than none.
func SummarizeTrace(output string) string {
	hists := ParseTraceHistograms(output)
	if len(hists) == 0 {
		return ""
	}

	all := TraceHistogram{Key: "all"}
	for _, hist := range hists {
		all.Buckets = mergeBuckets(all.Buckets, hist.Buckets)
		all.Total += hist.Total
	}

	slowest := hists[0]
	slowestMass := 0.0
	for _, hist := range hists {
		mass := hist.Mean() * float64(hist.Total)
		if mass > slowestMass {
			slowest, slowestMass = hist, mass
		}
	}

	share := 0.0
	if total := sumMass(hists); total > 0 {
		share = slowestMass / total * 100
	}

	var b strings.Builder

	fmt.Fprintf(&b, "延迟汇总（直方图分桶中点估算）：共 %d 次等待，p50 %s，p90 %s，p99 %s。",
		all.Total, formatMicros(all.Quantile(0.50)), formatMicros(all.Quantile(0.90)), formatMicros(all.Quantile(0.99)))

	if len(hists) > 1 || slowest.Key != "" {
		fmt.Fprintf(&b, "\n等待最久的是 %s：占 %.0f%% 的等待量，p99 %s。",
			orUnnamed(slowest.Key), share, formatMicros(slowest.Quantile(0.99)))
	}

	return b.String()
}

func orUnnamed(key string) string {
	if strings.TrimSpace(key) == "" {
		return "（未命名）"
	}

	return key
}

func sumMass(hists []TraceHistogram) float64 {
	total := 0.0
	for _, hist := range hists {
		total += hist.Mean() * float64(hist.Total)
	}

	return total
}

// mergeBuckets folds per-subject histograms into one. Keys sit in identical
// power-of-two ranges, but a range missing from one subject is still a range:
// the counts add up either way.
func mergeBuckets(into, from []TraceBucket) []TraceBucket {
	type key struct{ low, high float64 }

	counts := map[key]int{}
	for _, bucket := range into {
		counts[key{bucket.Low, bucket.High}] += bucket.Count
	}
	for _, bucket := range from {
		counts[key{bucket.Low, bucket.High}] += bucket.Count
	}

	merged := make([]TraceBucket, 0, len(counts))
	for k, count := range counts {
		merged = append(merged, TraceBucket{Low: k.low, High: k.high, Count: count})
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i].Low < merged[j].Low })

	return merged
}

func formatMicros(us float64) string {
	switch {
	case us >= 1e6:
		return fmt.Sprintf("%.2fs", us/1e6)
	case us >= 1e3:
		return fmt.Sprintf("%.2fms", us/1e3)
	}

	return fmt.Sprintf("%.1fµs", us)
}
