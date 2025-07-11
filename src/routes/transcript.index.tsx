import { createFileRoute, Link } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Search, FileText } from 'lucide-react'
import { ErrorDisplay } from '@/components/ErrorDisplay'

interface Session {
  sessionId: string
  messageCount: number
  startTime: string
  lastActivity: string
}

function TranscriptIndex() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [searchTerm, setSearchTerm] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void fetchSessions()
  }, [])

  const fetchSessions = () => {
    try {
      // This would be the actual API call when the endpoint exists
      // const response = await fetch('/v1/claude/sessions')
      // const data = await response.json()
      
      // Mock data for now
      const mockSessions: Session[] = [
        {
          sessionId: '5dd5fb40-6571-4cf3-a846-4e02a9c6dcad',
          messageCount: 45,
          startTime: '2025-07-10T23:08:24.376Z',
          lastActivity: '2025-07-10T23:12:52.629Z'
        }
      ]
      
      setSessions(mockSessions)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch sessions')
    } finally {
      setLoading(false)
    }
  }

  const filteredSessions = sessions.filter(session =>
    session.sessionId.toLowerCase().includes(searchTerm.toLowerCase())
  )

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-4xl mx-auto">
          <div className="flex items-center justify-center py-8">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-2">Loading sessions...</span>
          </div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="container mx-auto px-4 py-8">
        <div className="max-w-4xl mx-auto">
          <ErrorDisplay
            title="Failed to Load Sessions"
            message={error}
            onRetry={() => void fetchSessions()}
            retryLabel="Try Again"
          />
        </div>
      </div>
    )
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="max-w-4xl mx-auto space-y-6">
        <div>
          <h1 className="text-3xl font-bold mb-2">Claude Session Transcripts</h1>
          <p className="text-muted-foreground">
            Browse and view detailed transcripts of Claude coding sessions
          </p>
        </div>

        <div className="relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search sessions..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="pl-10"
          />
        </div>

        <div className="space-y-4">
          {filteredSessions.length === 0 ? (
            <Card>
              <CardContent className="p-6 text-center">
                <div className="text-muted-foreground">
                  {searchTerm ? 'No sessions match your search.' : 'No sessions available.'}
                </div>
              </CardContent>
            </Card>
          ) : (
            filteredSessions.map((session) => (
              <Card key={session.sessionId} className="hover:shadow-md transition-shadow">
                <CardHeader>
                  <CardTitle className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <FileText className="h-5 w-5" />
                      <span className="font-mono text-sm">
                        {session.sessionId}
                      </span>
                    </div>
                    <Link
                      to="/transcript/$sessionId"
                      params={{ sessionId: session.sessionId }}
                    >
                      <Button size="sm">
                        View Transcript
                      </Button>
                    </Link>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-4 text-sm text-muted-foreground">
                    <Badge variant="secondary">
                      {session.messageCount} messages
                    </Badge>
                    <span>
                      Started: {new Date(session.startTime).toLocaleString()}
                    </span>
                    <span>
                      Last activity: {new Date(session.lastActivity).toLocaleString()}
                    </span>
                  </div>
                </CardContent>
              </Card>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

export const Route = createFileRoute('/transcript/')({
  component: TranscriptIndex,
})