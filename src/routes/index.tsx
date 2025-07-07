import { createFileRoute, Link } from '@tanstack/react-router'

function Index() {
  return (
    <div className="container mx-auto px-4 py-12">
      <div className="max-w-2xl mx-auto text-center">
        <h1 className="text-4xl font-bold mb-6">Welcome to catnip</h1>
        <p className="text-lg text-muted-foreground mb-8">
          Making agentic coding fun and productive. Get started with a full
          terminal environment in your browser.
        </p>
        <div className="space-y-4">
          <Link
            to="/terminal"
            className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-8 py-2 text-sm font-medium text-primary-foreground ring-offset-background transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50"
          >
            Open Terminal
          </Link>
        </div>
        <div className="mt-12 grid gap-6 md:grid-cols-2">
          <div className="rounded-lg border p-6">
            <h3 className="font-semibold mb-2">Full Terminal Access</h3>
            <p className="text-sm text-muted-foreground">
              Complete bash environment with PTY support for running commands
              and interactive sessions.
            </p>
          </div>
          <div className="rounded-lg border p-6">
            <h3 className="font-semibold mb-2">Cloud or Local</h3>
            <p className="text-sm text-muted-foreground">
              Run locally with Vite or deploy to Cloudflare Containers for
              cloud-based development.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

export const Route = createFileRoute('/')({
  component: Index,
})