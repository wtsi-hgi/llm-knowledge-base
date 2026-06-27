'use client'

import { useActionState, useEffect } from 'react'
import { toast } from 'sonner'

import { requestGreeting } from '@/app/actions'
import { initialGreetingState } from '@/lib/greeting-state'
import { Button } from '@/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'

interface HelloFormProps {
	initialMessage: string
}

export function HelloForm({ initialMessage }: HelloFormProps) {
	const [state, formAction, pending] = useActionState(
		requestGreeting,
		initialGreetingState
	)

	useEffect(() => {
		if (state.status === 'success' && state.message) {
			toast.success('Greeting received from FastAPI')
		}
		if (state.status === 'error' && state.error) {
			toast.error(state.error)
		}
	}, [state])

	const latestMessage = state.message ?? initialMessage

	return (
		<Card className="shadow-lg">
			<CardHeader>
				<CardTitle>Request a greeting</CardTitle>
				<CardDescription>
					Server Actions invoke the FastAPI backend securely without exposing
					credentials in the browser.
				</CardDescription>
			</CardHeader>
			<CardContent>
				<form action={formAction} className="space-y-4">
					<div className="flex flex-col gap-3 sm:flex-row">
						<Input
							name="name"
							placeholder="Ada Lovelace"
							aria-label="Recipient name"
							autoComplete="off"
							disabled={pending}
						/>
						<Button type="submit" disabled={pending} className="shrink-0">
							{pending ? 'Contacting FastAPI…' : 'Ask FastAPI'}
						</Button>
					</div>
					<div className="rounded-lg border border-border bg-muted/40 p-4">
						<p className="text-sm font-semibold text-muted-foreground">
							Latest response
						</p>
						<p className="mt-2 text-base text-foreground">{latestMessage}</p>
						{state.status === 'error' && state.error && (
							<p className="mt-2 text-sm text-destructive">{state.error}</p>
						)}
					</div>
				</form>
			</CardContent>
		</Card>
	)
}
