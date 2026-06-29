import { z } from 'zod'

export const messageResponseSchema = z.object({
	message: z.string().min(1),
})

export type MessageResponse = z.infer<typeof messageResponseSchema>

export const healthResponseSchema = z.object({
	status: z.enum(['healthy', 'unhealthy']).or(z.string()),
})

export type HealthResponse = z.infer<typeof healthResponseSchema>

export const errorResponseSchema = z.object({
	message: z.string().optional(),
	detail: z.unknown().optional(),
})

export type ErrorResponse = z.infer<typeof errorResponseSchema>
