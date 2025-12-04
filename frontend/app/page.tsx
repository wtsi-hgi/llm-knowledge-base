'use client'

import { useState, useTransition } from 'react'
import { Button } from '@/components/ui/button'

export default function Home() {
	const [message, setMessage] = useState<string | null>(null)
	const [error, setError] = useState<string | null>(null)
	const [isPending, startTransition] = useTransition()

	function callBackend() {
		startTransition(async () => {
			setError(null)
			try {
				const response = await fetch('/api/hello')
				if (!response.ok) {
					throw new Error(`HTTP ${response.status}`)
				}
				const data = await response.json()
				setMessage(data.message ?? JSON.stringify(data))
			} catch (err) {
				setError(err instanceof Error ? err.message : 'Error calling backend')
				setMessage(null)
			}
		})
	}

	return (
		<main className="container mx-auto max-w-2xl px-4 py-10">
			<h1 className="text-2xl font-semibold tracking-tight">
				Hello World from Next.js + shadcn/ui
			</h1>
			<p className="mt-2 text-sm text-muted-foreground">
				This page uses a shadcn/ui Button component with Tailwind CSS v4.
			</p>
			<div className="mt-6">
				<Button onClick={callBackend} disabled={isPending}>
					{isPending ? 'Loading...' : 'Call FastAPI /hello'}
				</Button>
			</div>
			{message && (
				<div className="mt-4 rounded-lg border border-border bg-card p-4">
					<p className="text-sm font-medium">Backend response:</p>
					<p className="mt-1 text-muted-foreground">{message}</p>
				</div>
			)}
			{error && (
				<div className="mt-4 rounded-lg border border-destructive bg-destructive/10 p-4 text-destructive">
					<p className="text-sm font-medium">Error: {error}</p>
				</div>
			)}
		</main>
	)
}
