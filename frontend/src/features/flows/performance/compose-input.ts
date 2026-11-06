// A performance goal is three numbers a person knows before the run: which
// metric, what target, and what must not be touched. They are turned into a
// block appended to the flow input, because the planner reads the input text --
// there is no field for them in createFlow, and adding one would mean a schema,
// a migration and a resolver for three strings the model reads as prose anyway.

const MARKER = '【性能目标】';
const SECTION = /【[^】]{1,40}】/;

export interface GoalFields {
    goalConstraint?: string;
    goalMetric?: string;
    goalTarget?: string;
    message: string;
    targetType: string;
}

export interface PerformanceGoal {
    constraint?: string;
    metric?: string;
    target?: string;
}

// The three goal fields mean nothing once the flow is no longer an optimisation,
// so a value left over from a changed selection is dropped rather than shipped.
export function composeFlowInput(values: GoalFields): string {
    const isOptimisation = values.targetType === 'optimization';

    return composePerformanceInput(values.message.trim(), {
        constraint: isOptimisation ? values.goalConstraint : undefined,
        metric: isOptimisation ? values.goalMetric : undefined,
        target: isOptimisation ? values.goalTarget : undefined,
    });
}

export function composePerformanceInput(message: string, goal: PerformanceGoal = {}): string {
    const base = stripPerformanceGoal(message.trim());
    const metric = goal.metric?.trim() ?? '';
    const target = goal.target?.trim() ?? '';
    const constraint = goal.constraint?.trim() ?? '';

    if (!metric && !target && !constraint) {
        return base;
    }

    const lines = [MARKER];

    if (metric) {
        lines.push(`- 指标：${metric}`);
    }

    if (target) {
        lines.push(`- 目标：${target}`);
    }

    if (constraint) {
        lines.push(`- 约束：${constraint}`);
    }

    return `${base}\n\n${lines.join('\n')}`;
}

// Composing twice must not stack blocks: a retry of a flow that already carries
// one has to end up with a single copy of the current values.
export function stripPerformanceGoal(message: string): string {
    const start = message.indexOf(MARKER);

    if (start === -1) {
        return message;
    }

    const rest = message.slice(start + MARKER.length);
    const nextSection = rest.search(SECTION);
    const tail = nextSection === -1 ? '' : rest.slice(nextSection);

    return `${message.slice(0, start).trimEnd()}${tail ? `\n\n${tail.trim()}` : ''}`.trim();
}
