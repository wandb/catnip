import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/terminal/')({
  beforeLoad: ({ location }) => {
    // Get query parameters from the current location
    const searchParams = new URLSearchParams(location.search)
    const queryString = searchParams.toString()
    
    // Redirect to default session with preserved query parameters
    throw redirect({
      to: '/terminal/$sessionId',
      params: { sessionId: 'default' },
      search: queryString ? `?${queryString}` : undefined,
    })
  },
})