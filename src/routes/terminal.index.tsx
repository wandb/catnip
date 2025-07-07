import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/terminal/')({
  beforeLoad: async ({ location }) => {
    // Get query parameters from the current location
    const searchParams = new URLSearchParams(location.search)
    const queryString = searchParams.toString()
    
    // Redirect to default session with preserved query parameters
    throw redirect({
      to: '/terminal/default',
      search: queryString ? `?${queryString}` : undefined,
    })
  },
})