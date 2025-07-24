import { createFileRoute } from '@tanstack/react-router'
import { TranscriptExample } from '@/components/TranscriptExample'

function TranscriptDemo() {
  return <TranscriptExample />
}

export const Route = createFileRoute('/transcript/demo')({
  component: TranscriptDemo,
})