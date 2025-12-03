'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'

export default function Home() {
	const [msg, setMsg] = useState<string | null>(null)

	async function callBackend() {
		try {
			const res = await fetch('/api/hello')
			const data = await res.json()
			setMsg(data.message || JSON.stringify(data))
		} catch (err) {
			setMsg('Error calling backend')
		}
	}

	return (
		<main className="p-10 font-sans">
			<h1 className="text-2xl font-semibold">Hello World from Next.js + shadcn/ui</h1>
			<p className="mt-2 text-sm text-muted-foreground">
				This page uses a shadcn/ui Button component with Tailwind CSS v4.
			</p>
			<div className="mt-4">
				<Button onClick={callBackend}>Call FastAPI /hello</Button>
			</div>
			{msg && (
				<div className="mt-4">
					<strong>Backend response:</strong>
					<p className="mt-1">{msg}</p>
				</div>
			)}
		</main>
	)
}
