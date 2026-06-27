import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from '@/components/ui/card'

export default function Loading() {
	return (
		<main className="container mx-auto max-w-4xl px-4 py-12">
			<section className="space-y-4">
				<div className="h-4 w-32 animate-pulse rounded bg-muted" />
				<div className="h-12 w-3/4 animate-pulse rounded bg-muted" />
				<div className="h-6 w-1/2 animate-pulse rounded bg-muted" />
				<div className="flex items-center gap-3">
					<div className="h-8 w-24 animate-pulse rounded-full bg-muted" />
					<div className="h-4 w-40 animate-pulse rounded bg-muted" />
				</div>
			</section>
			<section className="mt-10">
				<Card className="shadow-lg">
					<CardHeader>
						<CardTitle>
							<div className="h-6 w-40 animate-pulse rounded bg-muted" />
						</CardTitle>
						<CardDescription>
							<div className="h-4 w-64 animate-pulse rounded bg-muted" />
						</CardDescription>
					</CardHeader>
					<CardContent>
						<div className="space-y-4">
							<div className="flex flex-col gap-3 sm:flex-row">
								<div className="h-10 flex-1 animate-pulse rounded bg-muted" />
								<div className="h-10 w-28 shrink-0 animate-pulse rounded bg-muted" />
							</div>
							<div className="h-24 animate-pulse rounded-lg bg-muted" />
						</div>
					</CardContent>
				</Card>
			</section>
		</main>
	)
}
