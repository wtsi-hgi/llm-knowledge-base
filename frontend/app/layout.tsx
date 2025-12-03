import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = {
	title: 'Next.js + shadcn/ui + FastAPI',
	description: 'A full-stack app with Next.js, shadcn/ui, and FastAPI',
}

export default function RootLayout({
	children,
}: {
	children: React.ReactNode
}) {
	return (
		<html lang="en">
			<body className="antialiased">{children}</body>
		</html>
	)
}
