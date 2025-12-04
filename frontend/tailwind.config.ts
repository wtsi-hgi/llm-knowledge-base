import type { Config } from 'tailwindcss'

const config: Config = {
	content: [
		'./pages/**/*.{js,ts,jsx,tsx,mdx}',
		'./components/**/*.{js,ts,jsx,tsx,mdx}',
		'./app/**/*.{js,ts,jsx,tsx,mdx}',
	],
	theme: {
		extend: {
			// Extend your theme here if needed
			// This config is optional with Tailwind v4's @theme inline
			// but useful for plugins and complex customizations
		},
	},
	plugins: [],
}

export default config
