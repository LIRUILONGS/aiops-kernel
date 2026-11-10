import { cn } from '@/lib/utils';

interface LogoProps extends React.SVGProps<SVGSVGElement> {
    className?: string;
}

const PULSE = 'M196 512 H360 L432 392 L520 648 L596 512 H828';
const RING_COLOUR = 'var(--logo-ring, currentColor)';
const PULSE_COLOUR = 'var(--logo-pulse, #fbbf24)';

function Logo({ className, ...props }: LogoProps) {
    return (
        <svg
            className={cn(className)}
            viewBox="0 0 1024 1024"
            xmlns="http://www.w3.org/2000/svg"
            {...props}
        >
            <circle
                cx="512"
                cy="512"
                fill="none"
                r="279"
                stroke={RING_COLOUR}
                strokeWidth="74"
            />
            <path
                d={PULSE}
                fill="none"
                stroke={PULSE_COLOUR}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="62"
            />
            <circle
                cx="196"
                cy="512"
                fill={PULSE_COLOUR}
                r="31"
            />
            <circle
                cx="828"
                cy="512"
                fill={PULSE_COLOUR}
                r="31"
            />
        </svg>
    );
}

export default Logo;
