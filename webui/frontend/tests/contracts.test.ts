import { describe, expect, it } from 'vitest'

import {
	healthResponseSchema,
	messageResponseSchema,
} from '@/lib/contracts'

describe('shared API contracts', () => {
	it('parses the hello message payload', () => {
		const payload = { message: 'Hello, test' }
		const parsed = messageResponseSchema.parse(payload)
		expect(parsed).toEqual(payload)
	})

	it('rejects malformed hello payloads', () => {
		const payload = { text: 'nope' }
		const result = messageResponseSchema.safeParse(payload)
		expect(result.success).toBe(false)
	})

	it('parses the health payload while allowing custom statuses', () => {
		const payload = { status: 'healthy' }
		expect(healthResponseSchema.parse(payload)).toEqual(payload)
	})
})
