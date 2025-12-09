'use client'

import { useEffect } from 'react'

import { Button } from '@/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from '@/components/ui/card'

export default function Error({
	error,
	reset,
}: {
	error: Error & { digest?: string }
	reset: () => void
}) {
	useEffect(() => {
		console.error('Application error:', error)
	}, [error])

	return (
		<main className="container mx-auto flex min-h-screen max-w-4xl items-center justify-center px-4">
			<Card className="w-full max-w-md shadow-lg">
				<CardHeader>
					<CardTitle className="text-destructive">Something went wrong</CardTitle>
					<CardDescription>
						An unexpected error occurred. Please try again.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{error.digest && (
						<p className="text-sm text-muted-foreground">
							Error ID: <code className="text-xs">{error.digest}</code>
						</p>
					)}
					<Button onClick={reset} variant="default">
						Try again
					</Button>
				</CardContent>
			</Card>
		</main>
	)
}
