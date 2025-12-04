import nextConfig from 'eslint-config-next'

const config = [
	...nextConfig,
	{
		ignores: ['.next/**', 'dist/**', 'node_modules/**'],
	},
]

export default config
