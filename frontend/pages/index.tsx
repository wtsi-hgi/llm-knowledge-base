import { useState } from 'react';
import Button from '../components/ui/button';

export default function Home() {
	const [msg, setMsg] = useState<string | null>(null);

	async function callBackend() {
		try {
			// call backend running on localhost:8000 in development
			const res = await fetch('http://localhost:8000/hello');
			const data = await res.json();
			setMsg(data.message || JSON.stringify(data));
		} catch (err) {
			setMsg('Error calling backend');
		}
	}

	return (
		<main style={{ padding: 40, fontFamily: 'Inter, system-ui, -apple-system, Roboto, sans-serif' }}>
			<h1 className="text-2xl font-semibold">Hello World from Next.js + shadcn-style UI</h1>
			<p className="mt-2 text-sm text-slate-600">This page uses a shadcn-style `components/ui/button.tsx` (Tailwind required).</p>
			<div className="mt-3">
				<Button onClick={callBackend}>Call FastAPI /hello</Button>
			</div>
			{msg && (
				<div style={{ marginTop: 16, color: '#111827' }}>
					<strong>Backend response:</strong>
					<div>{msg}</div>
				</div>
			)}
		</main>
	);
}
