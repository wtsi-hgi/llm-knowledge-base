import { NextRequest, NextResponse } from 'next/server'

/**
 * Backend URL configuration.
 * Uses BACKEND_URL env var if set, otherwise constructs from BACKEND_PORT.
 */
function getBackendUrl(): string {
	if (process.env.BACKEND_URL) {
		return process.env.BACKEND_URL
	}
	const port = process.env.BACKEND_PORT ?? '8000'
	return `http://localhost:${port}`
}

/**
 * API Route Handler: Proxies requests to the FastAPI backend.
 * This pattern keeps the backend URL internal and avoids CORS issues.
 */
export async function GET(request: NextRequest) {
	const backendUrl = getBackendUrl()

	try {
		const { searchParams } = new URL(request.url)
		const url = new URL('/api/v1/hello', backendUrl)

		// Forward query parameters
		const name = searchParams.get('name')
		if (name) {
			url.searchParams.set('name', name)
		}

		const response = await fetch(url.toString(), {
			method: 'GET',
			headers: {
				'Content-Type': 'application/json',
			},
			// Disable caching for API routes
			cache: 'no-store',
		})

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({}))
			return NextResponse.json(
				{ message: errorData.detail ?? 'Backend request failed' },
				{ status: response.status }
			)
		}

		const data = await response.json()
		return NextResponse.json(data)
	} catch (error) {
		console.error('Backend proxy error:', error)

		const message =
			error instanceof Error ? error.message : 'Failed to connect to backend'

		return NextResponse.json({ message }, { status: 503 })
	}
}
