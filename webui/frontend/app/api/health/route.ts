import { backendJson } from '@/lib/backend-client'
import { healthResponseSchema } from '@/lib/contracts'
import { NextResponse } from 'next/server'

export const dynamic = 'force-dynamic'

export async function GET() {
	try {
		const response = await backendJson('/api/v1/health', healthResponseSchema)
		return NextResponse.json(response)
	} catch (error) {
		console.error('Health check failed:', error)
		return NextResponse.json(
			{ status: 'unhealthy' },
			{ status: 503 }
		)
	}
}
