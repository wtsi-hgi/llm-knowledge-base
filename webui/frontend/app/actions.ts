'use server'

import { backendJson } from '@/lib/backend-client'
import {
	healthResponseSchema,
	messageResponseSchema,
} from '@/lib/contracts'
import { type GreetingState } from '@/lib/greeting-state'

export async function requestGreeting(
	_prevState: GreetingState,
	formData: FormData
): Promise<GreetingState> {
	const name = (formData.get('name') ?? 'World').toString() || 'World'

	try {
		const response = await backendJson(
			`/api/v1/hello?name=${encodeURIComponent(name)}`,
			messageResponseSchema
		)
		return {
			status: 'success',
			message: response.message,
			error: null,
		}
	} catch (error) {
		return {
			status: 'error',
			message: null,
			error: error instanceof Error ? error.message : 'Unknown error',
		}
	}
}

export async function fetchInitialGreeting() {
	return backendJson('/api/v1/hello', messageResponseSchema)
}

export async function fetchHealth() {
	return backendJson('/api/v1/health', healthResponseSchema)
}
