import { NextRequest, NextResponse } from 'next/server'
import { type ZodSchema } from 'zod'

import { buildBackendUrl } from '@/lib/backend-client'
import { errorResponseSchema } from '@/lib/contracts'

const forwardedHeaders = ['authorization', 'content-type', 'accept', 'cookie']

type ProxyOptions<T> = {
  backendPath: string | ((request: NextRequest) => string)
  schema?: ZodSchema<T>
  forwardSearchParams?: boolean
}

export async function proxyRequest<T>(
  request: NextRequest,
  options: ProxyOptions<T>
): Promise<NextResponse> {
  const backendPath =
    typeof options.backendPath === 'function'
      ? options.backendPath(request)
      : options.backendPath

  const url = buildBackendUrl(backendPath)

  if (options.forwardSearchParams !== false) {
    for (const [key, value] of request.nextUrl.searchParams.entries()) {
      if (!url.searchParams.has(key)) {
        url.searchParams.set(key, value)
      }
    }
  }

  const init: RequestInit = {
    method: request.method,
    headers: Object.fromEntries(
      forwardedHeaders
        .map((header) => [header, request.headers.get(header)])
        .filter(([, value]) => Boolean(value)) as Array<[string, string]>
    ),
    cache: 'no-store',
  }

  if (request.method !== 'GET' && request.method !== 'HEAD') {
    const body = await request.arrayBuffer()
    init.body = body
    // Needed for streaming requests in Edge runtime
    // @ts-expect-error -- duplex is still experimental in types
    init.duplex = 'half'
  }

  try {
    const response = await fetch(url, init)
    const contentType = response.headers.get('content-type') ?? ''
    const isJson = contentType.includes('application/json')
    const payload = isJson ? await response.json() : await response.text()

    if (!response.ok) {
      const parsed = isJson ? errorResponseSchema.safeParse(payload) : null
      const body = parsed?.success
        ? parsed.data
        : {
            message:
              typeof payload === 'string' ? payload : 'Backend request failed',
          }
      return NextResponse.json(body, { status: response.status })
    }

    if (!isJson) {
      return new NextResponse(payload as string, {
        status: response.status,
        headers: response.headers,
      })
    }

    if (options.schema) {
      const parsed = options.schema.safeParse(payload)
      if (!parsed.success) {
        return NextResponse.json(
          {
            message: 'Response validation failed',
            issues: parsed.error.issues,
          },
          { status: 502 }
        )
      }
      return NextResponse.json(parsed.data)
    }

    return NextResponse.json(payload)
  } catch (error) {
    console.error('Backend proxy error:', error)
    return NextResponse.json(
      { message: 'Failed to connect to backend' },
      { status: 503 }
    )
  }
}
