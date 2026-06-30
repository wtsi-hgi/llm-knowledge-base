export type GreetingState = {
	status: 'idle' | 'submitting' | 'success' | 'error'
	message: string | null
	error: string | null
}

export const initialGreetingState: GreetingState = {
	status: 'idle',
	message: null,
	error: null,
}
