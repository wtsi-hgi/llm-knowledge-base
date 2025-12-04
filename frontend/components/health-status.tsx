import { cn } from '@/lib/utils'
import { type HealthResponse } from '@/lib/contracts'

interface HealthStatusProps {
	status: HealthResponse['status']
}

export function HealthStatus({ status }: HealthStatusProps) {
	const normalized = status === 'healthy' ? 'healthy' : 'unhealthy'
	return (
		<span
			className={cn(
				'inline-flex items-center gap-2 rounded-full border px-3 py-1 text-sm font-medium uppercase tracking-wide',
				normalized === 'healthy'
					? 'border-emerald-200 bg-emerald-50 text-emerald-900'
					: 'border-destructive/40 bg-destructive/10 text-destructive'
			)}
		>
			<span
				className={cn('size-2 rounded-full', {
					'bg-emerald-500': normalized === 'healthy',
					'bg-destructive': normalized !== 'healthy',
				})}
			/>
			{normalized}
		</span>
	)
}
