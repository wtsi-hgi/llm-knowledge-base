import { HelloForm } from '@/components/hello-form'
import { HealthStatus } from '@/components/health-status'

import { fetchHealth, fetchInitialGreeting } from './actions'

export default async function Home() {
  const [greeting, health] = await Promise.all([
    fetchInitialGreeting(),
    fetchHealth(),
  ])

  return (
    <main className="container mx-auto max-w-4xl px-4 py-12">
      <section className="space-y-4">
        <p className="text-primary text-sm tracking-[0.3em] uppercase">
          Next.js + FastAPI
        </p>
        <h1 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">
          Full-stack starter with Server Actions and shadcn/ui
        </h1>
        <p className="text-muted-foreground max-w-2xl text-lg">
          This template calls FastAPI directly from the server layer, validates
          every payload with Zod, and keeps client components focused on UX.
        </p>
        <div className="flex items-center gap-3">
          <HealthStatus status={health.status} />
          <p className="text-muted-foreground text-sm">
            FastAPI `/api/v1/health` check
          </p>
        </div>
      </section>
      <section className="mt-10">
        <HelloForm initialMessage={greeting.message} />
      </section>
    </main>
  )
}
