import { NextRequest } from 'next/server'

import { proxyRequest } from '../_backendProxy'
import { messageResponseSchema } from '@/lib/contracts'

export async function GET(request: NextRequest) {
  return proxyRequest(request, {
    backendPath: '/api/v1/hello',
    schema: messageResponseSchema,
  })
}
