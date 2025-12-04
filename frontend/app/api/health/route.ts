import { NextRequest, NextResponse } from 'next/server'

function getBackendUrl(): string {
	if (process.env.BACKEND_URL) {
		return process.env.BACKEND_URL
	}
	const port = process.env.BACKEND_PORT ?? '8000'
	return `http://localhost:${port}`
}

export async function GET(request: NextRequest) {
	const backendUrl = getBackendUrl()

	try {
		const url = new URL('/api/v1/health', backendUrl)
		const response = await fetch(url.toString(), {
			method: 'GET',
			headers: {
				'Content-Type': 'application/json',
			},
			cache: 'no-store',
		})

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({}))
			return NextResponse.json(
				{ status: errorData.detail ?? 'unhealthy' },
				{ status: response.status }
			)
		}

		const data = await response.json()
		return NextResponse.json(data)
	} catch (error) {
		console.error('Backend health proxy error:', error)

		return NextResponse.json({ status: 'unhealthy' }, { status: 503 })
	}
}
