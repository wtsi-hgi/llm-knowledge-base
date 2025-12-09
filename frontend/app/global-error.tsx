'use client'

import { useEffect } from 'react'

export default function GlobalError({
	error,
	reset,
}: {
	error: Error & { digest?: string }
	reset: () => void
}) {
	useEffect(() => {
		console.error('Global error:', error)
	}, [error])

	return (
		<html lang="en">
			<body className="min-h-screen bg-neutral-50 font-sans antialiased">
				<main className="container mx-auto flex min-h-screen max-w-4xl items-center justify-center px-4">
					<div className="w-full max-w-md rounded-lg border border-neutral-200 bg-white p-6 shadow-lg">
						<h2 className="text-lg font-semibold text-red-600">
							Something went wrong
						</h2>
						<p className="mt-2 text-sm text-neutral-600">
							A critical error occurred. Please try again.
						</p>
						{error.digest && (
							<p className="mt-4 text-sm text-neutral-500">
								Error ID: <code className="text-xs">{error.digest}</code>
							</p>
						)}
						<button
							onClick={reset}
							className="mt-4 rounded-md bg-neutral-900 px-4 py-2 text-sm font-medium text-white hover:bg-neutral-800"
						>
							Try again
						</button>
					</div>
				</main>
			</body>
		</html>
	)
}
