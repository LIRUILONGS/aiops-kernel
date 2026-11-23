import { CapabilityMode } from '@/graphql/types';

const labels: Record<CapabilityMode, string> = {
    [CapabilityMode.Enforce]: '强制',
    [CapabilityMode.Off]: '关闭',
    [CapabilityMode.Shadow]: '观察',
};

export const capabilityModeLabel = (mode: CapabilityMode): string => labels[mode];
