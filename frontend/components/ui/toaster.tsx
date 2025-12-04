'use client'

import { Toaster as Sonner } from 'sonner'

export function Toaster() {
	return (
		<Sonner
			richColors
			position="top-right"
			className="toaster group"
			toastOptions={{
				className: 'group toast',
			}}
		/>
	)
}
