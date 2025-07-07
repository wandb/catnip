import { createFileRoute } from '@tanstack/react-router'
import { GitCheckout } from '@/components/GitCheckout'

export const Route = createFileRoute('/gh/$')({
  component: GitCheckout,
})