import { NextRequest } from 'next/server'

import { proxyRequest } from '../_backendProxy'
import { healthResponseSchema } from '@/lib/contracts'

export async function GET(request: NextRequest) {
  return proxyRequest(request, {
    backendPath: '/api/v1/health',
    schema: healthResponseSchema,
    forwardSearchParams: false,
  })
}
