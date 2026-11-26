import { TriangleAlert } from 'lucide-react';
import { useEffect, useSyncExternalStore } from 'react';
import { useRouteError } from 'react-router-dom';

import { Button } from '@/components/ui/button';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { isChunkLoadError, isDomDesyncError, reloadOnce } from '@/lib/chunk-reload';

const subscribeToConnection = (onChange: () => void) => {
    window.addEventListener('online', onChange);
    window.addEventListener('offline', onChange);

    return () => {
        window.removeEventListener('online', onChange);
        window.removeEventListener('offline', onChange);
    };
};

const isBrowserOnline = () => navigator.onLine;

const describeFailure = ({
    isChunk,
    isDesync,
    isOnline,
}: {
    isChunk: boolean;
    isDesync: boolean;
    isOnline: boolean;
}) => {
    if (isChunk && !isOnline) {
        return '当前处于离线状态，页面无法加载。网络恢复后会自动重新加载。';
    }

    if (isChunk) {
        return '可能刚刚部署了新版本，重新加载即可用上最新版本。';
    }

    if (isDesync) {
        return '页面出现了显示异常，重新加载通常可以恢复。';
    }

    return '页面遇到了意外错误，重新加载通常可以恢复。';
};

/**
 * Root `errorElement` for the data router — replaces React Router's built-in
 * dev crash screen. Two production failures land here: a code-split chunk that
 * 404s after a redeploy (auto-reload once, unless `vite:preloadError` in
 * `main.tsx` already did), and any other render/commit crash (e.g. a `removeChild`
 * desync from browser auto-translation) — both shown as a recoverable card.
 */
function RouteErrorBoundary() {
    const error = useRouteError();
    const isChunk = isChunkLoadError(error);
    const isDesync = isDomDesyncError(error);
    const isOnline = useSyncExternalStore(subscribeToConnection, isBrowserOnline);
    const isWaitingForConnection = isChunk && !isOnline;

    useEffect(() => {
        if (isChunk || isDesync) {
            reloadOnce();
        }
    }, [isChunk, isDesync, isOnline]);

    return (
        <div
            className="grid min-h-svh w-full place-items-center p-4"
            role="alert"
        >
            <Empty>
                <EmptyHeader>
                    <EmptyMedia variant="icon">
                        <TriangleAlert />
                    </EmptyMedia>
                    <EmptyTitle>出错了</EmptyTitle>
                    <EmptyDescription>{describeFailure({ isChunk, isDesync, isOnline })}</EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                    <Button
                        disabled={isWaitingForConnection}
                        onClick={() => window.location.reload()}
                        variant="secondary"
                    >
                        重新加载
                    </Button>
                </EmptyContent>
            </Empty>
        </div>
    );
}

export default RouteErrorBoundary;
