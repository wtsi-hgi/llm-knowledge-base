import { NextResponse } from 'next/server'

function getBackendUrl(): string {
	if (process.env.BACKEND_URL) {
		return process.env.BACKEND_URL
	}
	const port = process.env.BACKEND_PORT ?? '8000'
	return `http://localhost:${port}`
}

type ProxyOptions = {
	/**
	 * Optional function to transform the JSON body for non-OK responses.
	 * Allows each route to keep its specific error shape.
	 */
	mapErrorBody?: (json: unknown) => unknown
	/**
	 * Optional fallback body when the backend cannot be reached.
	 */
	fallbackBody?: unknown
}

export async function proxyGetJson(
	pathOrUrl: string | URL,
	options: ProxyOptions = {}
) {
	const backendUrl = getBackendUrl()

	try {
		const url =
			pathOrUrl instanceof URL ? pathOrUrl : new URL(pathOrUrl, backendUrl)

		const response = await fetch(url.toString(), {
			method: 'GET',
			headers: {
				'Content-Type': 'application/json',
			},
			cache: 'no-store',
		})

		if (!response.ok) {
			const raw = await response.json().catch(() => ({}))
			const body = options.mapErrorBody ? options.mapErrorBody(raw) : raw
			return NextResponse.json(body, { status: response.status })
		}

		const data = await response.json()
		return NextResponse.json(data)
	} catch (error) {
		console.error('Backend proxy error:', error)

		const body =
			options.fallbackBody ??
			(error instanceof Error
				? { message: error.message }
				: { message: 'Failed to connect to backend' })

		return NextResponse.json(body, { status: 503 })
	}
}
