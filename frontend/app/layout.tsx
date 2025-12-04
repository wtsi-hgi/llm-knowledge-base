import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'

const inter = Inter({
	subsets: ['latin'],
	variable: '--font-sans',
})

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
		<html lang="en" suppressHydrationWarning>
			<body className={`${inter.variable} min-h-screen bg-background font-sans text-foreground antialiased`}>
				{children}
			</body>
		</html>
	)
}
