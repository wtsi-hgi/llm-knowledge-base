import type { NextApiRequest, NextApiResponse } from 'next';

const BACKEND_PORT = process.env.BACKEND_PORT || '8000';
const BACKEND_URL = process.env.BACKEND_URL || `http://localhost:${BACKEND_PORT}`;

export default async function handler(
	req: NextApiRequest,
	res: NextApiResponse
) {
	try {
		// Forward the request to the FastAPI backend
		const url = new URL('/hello', BACKEND_URL);

		// Forward query parameters
		if (req.query.name) {
			url.searchParams.set('name', req.query.name as string);
		}

		const response = await fetch(url.toString(), {
			method: req.method,
			headers: {
				'Content-Type': 'application/json',
			},
		});

		const data = await response.json();
		res.status(response.status).json(data);
	} catch (error) {
		console.error('Backend proxy error:', error);
		res.status(500).json({ message: 'Failed to connect to backend' });
	}
}
