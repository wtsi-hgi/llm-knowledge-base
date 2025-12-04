import { NextRequest, NextResponse } from 'next/server'

const BACKEND_PORT = process.env.BACKEND_PORT || '8000'
const BACKEND_URL =
  process.env.BACKEND_URL || `http://localhost:${BACKEND_PORT}`

export async function GET(request: NextRequest) {
  try {
    const { searchParams } = new URL(request.url)
    const url = new URL('/hello', BACKEND_URL)

    const name = searchParams.get('name')
    if (name) {
      url.searchParams.set('name', name)
    }

    const response = await fetch(url.toString(), {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
      },
    })

    const data = await response.json()
    return NextResponse.json(data, { status: response.status })
  } catch (error) {
    console.error('Backend proxy error:', error)
    return NextResponse.json(
      { message: 'Failed to connect to backend' },
      { status: 500 }
    )
  }
}
