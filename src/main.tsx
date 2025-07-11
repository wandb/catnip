import { createRoot } from 'react-dom/client'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import './index.css'

// Import the generated route tree
import { routeTree } from './routeTree.gen'

// Create a new router instance with dynamic base path support
const basePath = (window as any).__PROXY_BASE_PATH__ || '/'
console.log('🔧 Router basePath detected:', basePath)
const router = createRouter({ 
  routeTree,
  basepath: basePath === '/' ? undefined : basePath.slice(0, -1) // Remove trailing slash for TanStack Router
})

// Register the router instance for type safety
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

createRoot(document.getElementById('root')!).render(
  <RouterProvider router={router} />
)
