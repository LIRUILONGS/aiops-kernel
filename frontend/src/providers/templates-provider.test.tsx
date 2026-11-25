import { ApolloClient, ApolloLink, Observable } from '@apollo/client';
import { ApolloProvider } from '@apollo/client/react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { createCache, watchQueryDefaults } from '@/lib/apollo';

import { TemplatesProvider, useTemplates } from './templates-provider';

vi.mock('@/providers/user-provider', () => ({
    useUser: () => ({ authInfo: { type: 'user' }, isAuthenticated: () => true }),
}));

const buildClient = (presets: Array<{ domain: string; text: string; title: string }>) =>
    new ApolloClient({
        cache: createCache(),
        defaultOptions: { watchQuery: watchQueryDefaults },
        link: new ApolloLink((operation) => {
            if (operation.operationName === 'flowTemplatePresets') {
                return new Observable<{ data: Record<string, unknown> }>((observer) => {
                    observer.next({ data: { flowTemplatePresets: presets } });
                    observer.complete();
                });
            }

            if (operation.operationName === 'flowTemplates') {
                return new Observable<{ data: Record<string, unknown> }>((observer) => {
                    observer.next({ data: { flowTemplates: [] } });
                    observer.complete();
                });
            }

            return new Observable<{ data: Record<string, unknown> }>(() => undefined);
        }),
    });

const PresetsProbe = () => {
    const { presets } = useTemplates();

    return (
        <ul>
            {presets.map((preset) => (
                <li key={preset.title}>
                    {preset.domain}:{preset.title}
                </li>
            ))}
        </ul>
    );
};

const renderPresets = (presets: Array<{ domain: string; text: string; title: string }>) =>
    render(
        <ApolloProvider client={buildClient(presets)}>
            <TemplatesProvider>
                <PresetsProbe />
            </TemplatesProvider>
        </ApolloProvider>,
    );

describe('TemplatesProvider presets', () => {
    it('hands the shipped presets to the form as domain and title', async () => {
        renderPresets([{ domain: '告警诊断', text: '用 ops_system 取负载', title: 'CPU 饱和告警' }]);

        expect(await screen.findByText('告警诊断:CPU 饱和告警')).toBeInTheDocument();
    });

    it('renders no preset when the server answers with an empty catalogue', async () => {
        renderPresets([]);

        expect(await screen.findByRole('list')).toBeEmptyDOMElement();
    });
});
