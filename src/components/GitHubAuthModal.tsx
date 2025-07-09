import { useState, useEffect } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Loader2, Copy, CheckCircle2, XCircle, Github } from 'lucide-react'

interface GitHubAuthModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface AuthResponse {
  code: string
  url: string
  status: string
}

interface AuthStatus {
  status: 'none' | 'pending' | 'waiting' | 'success' | 'error'
  error?: string
}

export function GitHubAuthModal({ open, onOpenChange }: GitHubAuthModalProps) {
  const [authData, setAuthData] = useState<AuthResponse | null>(null)
  const [status, setStatus] = useState<AuthStatus>({ status: 'none' })
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState(false)
  const [polling, setPolling] = useState(false)

  // Start auth flow when modal opens
  useEffect(() => {
    if (open && !authData) {
      startAuth()
    }
  }, [open])

  // Poll for status when waiting
  useEffect(() => {
    let interval: number | null = null
    
    if (polling && status.status === 'waiting') {
      interval = window.setInterval(async () => {
        try {
          const response = await fetch('/v1/auth/github/status')
          const data: AuthStatus = await response.json()
          
          if (data.status !== 'waiting') {
            setStatus(data)
            setPolling(false)
          }
        } catch (error) {
          console.error('Failed to check auth status:', error)
        }
      }, 2000) // Poll every 2 seconds
    }

    return () => {
      if (interval) {
        clearInterval(interval)
      }
    }
  }, [polling, status.status])

  const startAuth = async () => {
    setLoading(true)
    setAuthData(null)
    setStatus({ status: 'pending' })
    
    try {
      // Add timeout to the fetch request
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 15000) // 15 second timeout
      
      const response = await fetch('/v1/auth/github/start', {
        method: 'POST',
        signal: controller.signal,
      })
      
      clearTimeout(timeoutId)
      
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP ${response.status}: Failed to start authentication`)
      }
      
      const data: AuthResponse = await response.json()
      setAuthData(data)
      setStatus({ status: data.status as AuthStatus['status'] })
    } catch (error) {
      let errorMessage = 'Failed to start authentication'
      
      if (error instanceof Error) {
        if (error.name === 'AbortError') {
          errorMessage = 'Request timed out - please try again'
        } else {
          errorMessage = error.message
        }
      }
      
      setStatus({ 
        status: 'error', 
        error: errorMessage
      })
    } finally {
      setLoading(false)
    }
  }

  const copyCode = () => {
    if (authData?.code) {
      navigator.clipboard.writeText(authData.code)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  const openGitHub = () => {
    if (authData?.url) {
      window.open(authData.url, '_blank')
      setStatus({ status: 'waiting' })
      setPolling(true)
    }
  }

  const handleClose = () => {
    if (status.status === 'success' || status.status === 'error') {
      onOpenChange(false)
      // Reset state after closing
      setTimeout(() => {
        setAuthData(null)
        setStatus({ status: 'none' })
        setPolling(false)
      }, 300)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>GitHub Authentication</DialogTitle>
          <DialogDescription>
            Authenticate with GitHub to enable repository access and CLI features
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {loading && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          )}

          {!loading && authData && (status.status === 'pending' || status.status === 'waiting') && (
            <div className="space-y-4">
              <div className="rounded-lg border bg-muted p-4">
                <p className="mb-2 text-sm text-muted-foreground">Your one-time code:</p>
                <div className="flex items-center justify-between">
                  <code className="text-2xl font-bold tracking-wider">{authData.code}</code>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={copyCode}
                    className="ml-4"
                  >
                    {copied ? (
                      <>
                        <CheckCircle2 className="mr-2 h-4 w-4" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="mr-2 h-4 w-4" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
              </div>

              <Button onClick={openGitHub} className="w-full">
                <Github className="mr-2 h-4 w-4" />
                Login with GitHub
              </Button>
            </div>
          )}

          {status.status === 'waiting' && polling && (
            <div className="space-y-4">
              <Alert>
                <Loader2 className="h-4 w-4 animate-spin" />
                <AlertDescription>
                  Waiting for authentication to complete...
                </AlertDescription>
              </Alert>
              <p className="text-sm text-muted-foreground text-center">
                Complete the authentication process in your browser
              </p>
            </div>
          )}

          {status.status === 'success' && (
            <div className="space-y-4">
              <Alert className="border-green-600 bg-green-50 dark:bg-green-950">
                <CheckCircle2 className="h-4 w-4 text-green-600" />
                <AlertDescription className="text-green-800 dark:text-green-200">
                  Successfully authenticated with GitHub!
                </AlertDescription>
              </Alert>
              <Button onClick={handleClose} className="w-full">
                Close
              </Button>
            </div>
          )}

          {status.status === 'error' && (
            <div className="space-y-4">
              <Alert className="border-destructive">
                <XCircle className="h-4 w-4 text-destructive" />
                <AlertDescription className="text-destructive">
                  {status.error || 'Authentication failed'}
                </AlertDescription>
              </Alert>
              <div className="flex gap-2">
                <Button onClick={startAuth} variant="outline" className="flex-1">
                  Try Again
                </Button>
                <Button onClick={handleClose} variant="outline" className="flex-1">
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}