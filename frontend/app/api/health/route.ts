import { proxyGetJson } from '../_backendProxy'

export async function GET() {
  return proxyGetJson('/api/v1/health', {
    mapErrorBody: (raw: any) => ({
      status: raw?.detail ?? 'unhealthy',
    }),
    fallbackBody: { status: 'unhealthy' },
  })
}
