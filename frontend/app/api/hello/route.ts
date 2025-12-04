import { NextRequest } from 'next/server'

import { proxyGetJson } from '../_backendProxy'

/**
 * API Route Handler: Proxies requests to the FastAPI backend.
 * This pattern keeps the backend URL internal and avoids CORS issues.
 */
export async function GET(request: NextRequest) {
  const { searchParams } = new URL(request.url)
  const url = new URL('/api/v1/hello', 'http://placeholder')

  // Forward query parameters
  const name = searchParams.get('name')
  if (name) {
    url.searchParams.set('name', name)
  }

  // Only the path and query are used by the proxy helper,
  // the backend origin is resolved there.
  url.protocol = ''
  url.host = ''

  return proxyGetJson(`${url.pathname}${url.search}`, {
    mapErrorBody: (raw: any) => ({
      message: raw?.detail ?? 'Backend request failed',
    }),
    fallbackBody: { message: 'Failed to connect to backend' },
  })
}
