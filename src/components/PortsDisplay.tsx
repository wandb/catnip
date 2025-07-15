import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { ExternalLink, Globe, Server, Eye } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { useAppStore } from '../stores/appStore'

export function PortsDisplay() {
  const { getActivePorts, sseConnected } = useAppStore()
  const activePorts = getActivePorts()

  const getServiceIcon = (serviceType: string) => {
    switch (serviceType) {
      case 'http':
        return <Globe className="h-4 w-4" />
      case 'tcp':
        return <Server className="h-4 w-4" />
      default:
        return <Server className="h-4 w-4" />
    }
  }

  const openService = (port: number) => {
    window.open(`/${port}/`, '_blank')
  }

  const portCount = activePorts.length

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Server className="h-5 w-5" />
          Active Ports
          <Badge variant="secondary">{portCount}</Badge>
          {!sseConnected && (
            <Badge variant="destructive">Disconnected</Badge>
          )}
        </CardTitle>
        <CardDescription>
          Services automatically detected and available for proxy
        </CardDescription>
      </CardHeader>
      <CardContent>
        {portCount === 0 ? (
          <p className="text-muted-foreground text-center py-8">
            No active ports detected. Start a service to see it here automatically.
          </p>
        ) : (
          <div className="space-y-4">
            {activePorts.map((port) => (
              <div
                key={port.port}
                className="flex items-center justify-between p-4 border rounded-lg hover:bg-muted/50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  {getServiceIcon(port.service || 'unknown')}
                  <div>
                    <div className="font-medium">
                      Port {port.port}
                      {port.title && (
                        <span className="text-sm text-muted-foreground ml-2">
                          • {port.title}
                        </span>
                      )}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {(port.service || 'unknown').toUpperCase()} service
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">
                    {port.service || 'unknown'}
                  </Badge>
                  {port.service === 'http' && (
                    <div className="flex gap-1">
                      <Link to="/preview/$port" params={{ port: port.port.toString() }}>
                        <Button
                          size="sm"
                          variant="outline"
                          className="gap-1"
                        >
                          <Eye className="h-3 w-3" />
                          Preview
                        </Button>
                      </Link>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => openService(port.port)}
                        className="gap-1"
                      >
                        <ExternalLink className="h-3 w-3" />
                        Open
                      </Button>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}