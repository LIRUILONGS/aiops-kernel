package opsremote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func verifyFingerprint(host string, load, cpu float64, blocked int) Fingerprint {
	fp := Fingerprint{
		Label:      "sample",
		Host:       host,
		Load1:      load,
		CPUPercent: cpu,
		Containers: map[string]FingerprintContainer{},
	}
	if blocked >= 0 {
		fp.Postgres = &PostgresBrief{BlockedSessions: blocked}
	}

	return fp
}

func TestOpsRemote_JudgeChange_SaysUnverifiedWhenItCannotCompare(t *testing.T) {
	t.Parallel()

	before := verifyFingerprint("lab", 1, 10, 3)
	after := verifyFingerprint("lab", 1, 10, 0)

	cases := []struct {
		name   string
		before *Fingerprint
		after  *Fingerprint
		wants  string
	}{
		{name: "no baseline was taken", before: nil, after: &after, wants: "变更前"},
		{name: "the second collection failed", before: &before, after: nil, wants: "变更后"},
		{
			name:   "two different hosts",
			before: &before,
			after:  func() *Fingerprint { fp := verifyFingerprint("other", 1, 10, 0); return &fp }(),
			wants:  "同一台主机",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			change := JudgeChange(tc.before, tc.after, []string{"容器 api-1"})

			assert.Equal(t, VerdictUnverified, change.State)
			assert.False(t, change.Verified())
			assert.Contains(t, change.Reason, tc.wants)
		})
	}
}

func TestOpsRemote_JudgeChange_ReadsTheDirectionFromTheMaterialRows(t *testing.T) {
	t.Parallel()

	baseline := verifyFingerprint("lab", 8, 90, 4)

	t.Run("every material row improved", func(t *testing.T) {
		t.Parallel()

		after := verifyFingerprint("lab", 0.5, 12, 0)
		change := JudgeChange(&baseline, &after, []string{"锁等待会话", "最长锁等待", "长事务会话", "load1", "load5", "cpu", "内存占用", "swap 使用"})

		assert.Equal(t, VerdictImproved, change.State)
		assert.True(t, change.Verified())
		assert.NotEmpty(t, change.Deltas)
	})

	t.Run("one row got worse", func(t *testing.T) {
		t.Parallel()

		after := verifyFingerprint("lab", 0.4, 12, 9)
		change := JudgeChange(&baseline, &after, []string{"锁等待会话", "最长锁等待", "长事务会话", "load1", "load5", "cpu", "内存占用", "swap 使用"})

		assert.Equal(t, VerdictWorsened, change.State)
	})

	t.Run("nothing moved past the floor", func(t *testing.T) {
		t.Parallel()

		after := verifyFingerprint("lab", 8.01, 90, 4)
		change := JudgeChange(&baseline, &after, []string{"锁等待会话", "最长锁等待", "长事务会话", "load1", "load5", "cpu", "内存占用", "swap 使用"})

		assert.Equal(t, VerdictUnchanged, change.State)
		assert.True(t, change.Verified(), "no movement is still an answer, not a missing measurement")
		assert.Empty(t, change.Deltas, "noise must not be quoted as evidence")
	})
}

func TestOpsRemote_JudgeChange_RefusesToCallAStateChangeAnImprovement(t *testing.T) {
	t.Parallel()

	before := verifyFingerprint("lab", 1, 10, -1)
	before.Containers["api-1"] = FingerprintContainer{State: "exited"}
	after := verifyFingerprint("lab", 1, 10, -1)
	after.Containers["api-1"] = FingerprintContainer{State: "running"}

	change := JudgeChange(&before, &after, []string{"容器 api-1"})

	assert.Equal(t, VerdictChanged, change.State)
	assert.True(t, change.Verified())
	assert.Len(t, change.Deltas, 1)
	assert.Contains(t, change.Deltas[0].Field, "api-1")
}

func TestOpsRemote_JudgeChange_KeepsTheLabelsItCompared(t *testing.T) {
	t.Parallel()

	before := verifyFingerprint("lab", 1, 10, -1)
	before.Label = "before-change"
	before.CapturedAt = time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	after := verifyFingerprint("lab", 1, 10, -1)
	after.Label = "after-change"

	change := JudgeChange(&before, &after, []string{"容器 api-1"})

	assert.Equal(t, "before-change", change.BeforeLabel)
	assert.Equal(t, "after-change", change.AfterLabel)
}

func TestOpsRemote_JudgeChange_IgnoresWhatTheChangeCannotHaveTouched(t *testing.T) {
	t.Parallel()

	before := verifyFingerprint("lab", 1, 10, -1)
	before.Containers["api-1"] = FingerprintContainer{State: "exited"}
	after := verifyFingerprint("lab", 40, 99, -1)
	after.Containers["api-1"] = FingerprintContainer{State: "running"}

	change := JudgeChange(&before, &after, []string{"容器 api-1"})

	if change.State != VerdictChanged {
		t.Fatalf("state = %q, want %q: the host's load is not this restart's doing", change.State, VerdictChanged)
	}
	for _, delta := range change.Deltas {
		assert.NotEqual(t, "cpu", delta.Field, "a host metric counted as evidence of a container restart")
	}
	assert.NotEmpty(t, change.Bystanders, "the host metrics must still be shown, just not counted")
}

func TestOpsRemote_JudgeChange_SaysUnverifiedWhenTheFingerprintHasNoFaceForTheChange(t *testing.T) {
	t.Parallel()

	before := verifyFingerprint("lab", 1, 10, -1)
	after := verifyFingerprint("lab", 1, 10, -1)

	change := JudgeChange(&before, &after, nil)

	assert.Equal(t, VerdictUnverified, change.State)
	assert.Contains(t, change.Reason, "指纹")
}

func TestOpsRemote_Scope_NamesTheRowsAnActionCanMove(t *testing.T) {
	t.Parallel()

	prefixes, reason := Scope("container_restart", map[string]string{"Container": "api-1"})
	assert.Equal(t, []string{"容器 api-1"}, prefixes)
	assert.Empty(t, reason)

	disk, _ := Scope("disk_cleanup", map[string]string{"Path": "/var/log/nginx/"})
	assert.Equal(t, []string{"磁盘 "}, disk)

	for _, id := range []string{"unit_restart", "config_restore", "k8s_rollout_restart"} {
		none, why := Scope(id, nil)
		assert.Empty(t, none, "%s must not claim a fingerprint face it does not have", id)
		assert.NotEmpty(t, why)
	}
}
