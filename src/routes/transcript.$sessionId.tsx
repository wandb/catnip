import { createFileRoute } from '@tanstack/react-router'
import { TranscriptViewer } from '@/components/TranscriptViewer'

function TranscriptPage() {
  const { sessionId } = Route.useParams()
  
  return (
    <div className="container mx-auto px-4 py-8">
      <TranscriptViewer sessionId={sessionId} />
    </div>
  )
}

export const Route = createFileRoute('/transcript/$sessionId')({
  component: TranscriptPage,
})