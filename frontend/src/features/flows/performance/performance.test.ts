import { describe, expect, it } from 'vitest';

import { parseBaseline, parseDiff } from './performance';

// Both fixtures are real answers from the lab host, pasted verbatim: the parsers
// exist because these two tools answer in prose plus a table, not in JSON.
const BASELINE_RESULT = `已采集基线 "subtask68-anchor"（目标 lab，主机 iZ2zed2esntmi7pbsdfurlZ）
负载 0.04，CPU 2.0%，内存 546/1740 MB，磁盘根分区 12%

把下面这一整行原样保存下来，做完变更后再次调用 ops_baseline 采一个 after，再用 ops_diff 对比：
{"label":"subtask68-anchor","captured_at":"2026-11-06T12:46:29.9366986Z","host":"iZ2zed2esntmi7pbsdfurlZ","load1":0.04,"load5":0.07,"load15":0.08,"cpu_percent":2,"mem_used_mb":546,"mem_total_mb":1740,"swap_used_mb":0,"filesystems":{"/":12,"/run":1},"containers":{"caddy":{"state":"running","restarts":0,"exit_code":0},"opslab-broken":{"state":"restarting","restarts":428,"exit_code":1}}}`;

const DIFF_RESULT = `对比：before（2026-11-06 12:55:15）→ after（2026-11-06 12:55:20）
共 3 项，其中显著变化 1 项

指标	变化前	变化后	变化量	判定
load1	0.1	0.1	-0.0 (-7%)	持平
cpu	1.5%	3.0%	+1.5% (+98%)	持平
容器 opslab-broken 重启	464	428	-36 (-8%)	改善

没有显著变化：所有指标都在噪声范围内，本次变更未产生可测量的效果。`;

describe('parseBaseline', () => {
    it('lifts the fingerprint out of the prose around it', () => {
        const sample = parseBaseline(BASELINE_RESULT);

        expect(sample).not.toBeNull();
        expect(sample?.label).toBe('subtask68-anchor');
        expect(sample?.capturedAt).toBe('2026-11-06T12:46:29.9366986Z');
        expect(sample?.load1).toBeCloseTo(0.04);
        expect(sample?.cpuPercent).toBe(2);
        expect(sample?.memUsedMB).toBe(546);
        expect(sample?.memPercent).toBeCloseTo(31.38, 1);
    });

    it('counts restarts across containers and finds the fullest filesystem', () => {
        const sample = parseBaseline(BASELINE_RESULT);

        expect(sample?.restarts).toBe(428);
        expect(sample?.worstDiskPercent).toBe(12);
        expect(sample?.worstDiskPath).toBe('/');
    });

    it('answers nothing when the answer holds no fingerprint', () => {
        expect(parseBaseline('未采集基线：目标主机不可达')).toBeNull();
        expect(parseBaseline('')).toBeNull();
        expect(parseBaseline(undefined)).toBeNull();
    });
});

describe('parseDiff', () => {
    it('reads the two labels and the materiality count', () => {
        const diff = parseDiff(DIFF_RESULT);

        expect(diff).not.toBeNull();
        expect(diff?.beforeLabel).toBe('before');
        expect(diff?.afterLabel).toBe('after');
        expect(diff?.beforeTime).toBe('2026-11-06 12:55:15');
        expect(diff?.total).toBe(3);
        expect(diff?.material).toBe(1);
    });

    it('keeps every row with its verdict and materiality', () => {
        const diff = parseDiff(DIFF_RESULT);

        expect(diff?.rows.map((row) => [row.field, row.after, row.verdict, row.worsened])).toEqual([
            ['load1', '0.1', '持平', false],
            ['cpu', '3.0%', '持平', false],
            ['容器 opslab-broken 重启', '428', '改善', false],
        ]);
    });

    it('marks the row ops_diff flagged as getting worse', () => {
        const diff = parseDiff(`对比：before（2026-11-06 12:55:15）→ after（2026-11-06 12:56:15）
共 1 项，其中显著变化 1 项

指标	变化前	变化后	变化量	判定
容器 opslab-slow 重启	0	7	+7	恶化 ←
`);

        expect(diff?.rows[0]?.worsened).toBe(true);
        expect(diff?.rows[0]?.verdict).toBe('恶化');
    });

    it('keeps the closing sentence as the summary', () => {
        const diff = parseDiff(DIFF_RESULT);

        expect(diff?.summary).toContain('噪声范围内');
    });

    it('answers nothing when the answer is not a comparison', () => {
        expect(parseDiff('两次采样来自不同主机，无法对比')).toBeNull();
        expect(parseDiff(undefined)).toBeNull();
    });
});
