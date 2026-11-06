// A stand-in LLM for the acceptance run.
//
// The point of this file is not to be clever: it is to answer the AIOps prompts
// with the tool calls a real model would make, so that one command can prove the
// whole path -- flow, planner, commander, exit validation, reporter -- still
// works. Every answer is matched from the request JSON and is deliberately
// minimal: one subtask, one diagnosis, no host access.
//
//   node scripts/aiops_acceptance/mock-llm.mjs 41499
//
// Unmatched requests are written to the log with their first prompt line, so a
// prompt that gains a new marker shows up as a missing rule rather than as a
// mysteriously failing flow.

import { createServer } from 'node:http';

const PORT = Number(process.argv[2] || 41499);
const VERBOSE = process.env.MOCK_LLM_VERBOSE === '1';

const DIAGNOSIS = {
    blast_radius: 'only the acceptance host',
    confidence: 0.8,
    evidence: [
        {
            excerpt: 'container state=running restarts=0',
            query: 'ops_containers',
            source: 'containers',
            timestamp: '2026-01-01T00:00:00Z',
        },
        {
            excerpt: 'no error lines in the last 20 lines',
            query: 'ops_logs tail=20',
            source: 'logs',
            timestamp: '2026-01-01T00:00:01Z',
        },
    ],
    message: 'Acceptance diagnosis',
    needs_human: false,
    recommendation: {
        action: 'none, this is the mock',
        risk: 'low',
        rollback: '无需撤销',
    },
    root_cause: 'The acceptance run does not investigate anything: it proves the exit path.',
};

const PLAN = {
    message: 'Acceptance plan',
    subtasks: [
        {
            description: 'Answer from the recorded evidence, without touching a host.',
            mode: 'diagnosis',
            title: 'Diagnose the acceptance host',
        },
    ],
};

const REPORT = {
    message: 'Acceptance flow finished',
    result: 'The acceptance run reached its conclusion through the AIOps exit.',
    success: true,
};

const RULES = [
    { content: 'Acceptance Flow', label: 'flow-title', match: /Flow Title Generator/ },
    { content: 'Diagnose the acceptance host', label: 'task-title', match: /Task Title Generator/ },
    { content: 'English', label: 'language-chooser', match: /Language Detector/ },
    { content: 'debian:stable-slim', label: 'image-chooser', match: /Docker Image Selector/ },
    { content: 'call_000000', label: 'toolcall-pattern-detector', match: /submit_pattern/ },
    {
        label: 'toolcall-id-sample',
        match: /get_number/,
        toolCalls: [{ args: { value: 1 }, name: 'get_number' }],
    },
    {
        label: 'subtask-refine',
        match: /"subtask_patch"/,
        toolCalls: [{ args: { operations: [] }, name: 'subtask_patch' }],
    },
    {
        label: 'subtask-plan',
        match: /"subtask_list"/,
        toolCalls: [{ args: PLAN, name: 'subtask_list' }],
    },
    {
        label: 'diagnosis-exit',
        match: /"diagnosis"/,
        toolCalls: [{ args: DIAGNOSIS, name: 'diagnosis' }],
    },
    {
        label: 'task-report',
        match: /"report_result"/,
        toolCalls: [{ args: REPORT, name: 'report_result' }],
    },
];

function answer(body) {
    const text = JSON.stringify(body);

    for (const rule of RULES) {
        if (rule.match.test(text)) {
            return rule;
        }
    }

    return null;
}

function reply(message, finishReason = 'stop') {
    return {
        choices: [
            {
                finish_reason: finishReason,
                index: 0,
                message,
            },
        ],
        created: Math.floor(Date.now() / 1000),
        id: `mock-${Date.now()}`,
        model: 'mock-llm',
        object: 'chat.completion',
        usage: { completion_tokens: 8, prompt_tokens: 64, total_tokens: 72 },
    };
}

function toResponse(rule) {
    if (rule.toolCalls) {
        return reply(
            {
                content: null,
                role: 'assistant',
                tool_calls: rule.toolCalls.map((call, index) => ({
                    function: { arguments: JSON.stringify(call.args), name: call.name },
                    id: `call_10000${index}`,
                    type: 'function',
                })),
            },
            'tool_calls',
        );
    }

    return reply({ content: rule.content, role: 'assistant' });
}

const server = createServer((request, response) => {
    let raw = '';

    request.on('data', (chunk) => {
        raw += chunk;
    });

    request.on('end', () => {
        if (request.url?.startsWith('/v1/models')) {
            response.writeHead(200, { 'Content-Type': 'application/json' });
            response.end(
                JSON.stringify({
                    data: [{ id: 'mock-llm', object: 'model', owned_by: 'acceptance' }],
                    object: 'list',
                }),
            );

            return;
        }

        let body = {};

        try {
            body = JSON.parse(raw);
        } catch {
            body = {};
        }

        const rule = answer(body);

        if (VERBOSE) {
            const messages = body.messages || [];
            const last = messages.filter((item) => item.role !== 'system').at(-1) || {};
            const preview = String(last.content ?? '').slice(0, 120).replaceAll('\n', ' ');

            console.log(`[mock] ${rule ? rule.label : 'UNMATCHED'} :: ${preview}`);
        }

        if (!rule) {
            // An unmatched request still has to answer something, and plain text
            // makes the failure show up as a stalled chain, not a failed one.
            response.writeHead(200, { 'Content-Type': 'application/json' });
            response.end(
                JSON.stringify(reply({ content: 'mock: no rule matched this prompt', role: 'assistant' })),
            );

            return;
        }

        response.writeHead(200, { 'Content-Type': 'application/json' });
        response.end(JSON.stringify(toResponse(rule)));
    });
});

server.listen(PORT, '127.0.0.1', () => {
    console.log(`[mock] listening on http://127.0.0.1:${PORT}/v1`);
});
