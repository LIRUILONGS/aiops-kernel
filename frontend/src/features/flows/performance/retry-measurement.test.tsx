import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const navigate = vi.fn();
const createFlow = vi.fn();
const flowState = vi.hoisted(() => ({
    value: {
        flowData: {
            flow: { id: '12', provider: { name: 'custom', type: 'custom' } },
            tasks: [{ id: '10', input: '把 caddy 容器的 p99 延迟降下来' }],
        },
    } as { flowData?: { flow: { id: string; provider: { name: string } }; tasks: { id: string; input: string }[] } },
}));

vi.mock('react-router-dom', () => ({ useNavigate: () => navigate }));
vi.mock('@/providers/flow-provider', () => ({ useFlow: () => flowState.value }));
vi.mock('@/providers/flows-provider', () => ({ useFlows: () => ({ createFlow }) }));

import RetryMeasurement from './retry-measurement';

describe('RetryMeasurement', () => {
    beforeEach(() => {
        createFlow.mockReset();
        createFlow.mockResolvedValue('42');
        navigate.mockReset();
        flowState.value = {
            flowData: {
                flow: { id: '12', provider: { name: 'custom' } },
                tasks: [{ id: '10', input: '把 caddy 容器的 p99 延迟降下来' }],
            },
        };
    });

    it('offers a retry when the change was not re-measured', () => {
        render(<RetryMeasurement verdict="unverified" />);

        expect(screen.getByRole('button', { name: /重新测量/ })).toBeEnabled();
    });

    it('offers a retry when the change made things worse', () => {
        render(<RetryMeasurement verdict="degraded" />);

        expect(screen.getByRole('button', { name: /重新测量/ })).toBeEnabled();
    });

    it('offers nothing when the change worked', () => {
        render(<RetryMeasurement verdict="improved" />);

        expect(screen.queryByRole('button', { name: /重新测量/ })).not.toBeInTheDocument();
    });

    it('starts a new optimisation with the same input and goes to it', async () => {
        render(<RetryMeasurement verdict="no_measurable_effect" />);

        screen.getByRole('button', { name: /重新测量/ }).click();

        await waitFor(() => expect(navigate).toHaveBeenCalledWith('/flows/42'));
        expect(createFlow).toHaveBeenCalledWith({
            capabilityMode: 'off',
            message: '把 caddy 容器的 p99 延迟降下来',
            providerName: 'custom',
            resourceIds: [],
            targetType: 'optimization',
        });
    });

    it('stays disabled and does nothing when the flow has no input to repeat', () => {
        flowState.value = { flowData: undefined };

        render(<RetryMeasurement verdict="unverified" />);

        const button = screen.getByRole('button', { name: /重新测量/ });
        expect(button).toBeDisabled();

        button.click();
        expect(createFlow).not.toHaveBeenCalled();
    });
});
