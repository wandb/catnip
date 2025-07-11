import { useState, useEffect } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { ExternalLink, Globe, Server, Loader2, Eye } from 'lucide-react'
import { Link } from '@tanstack/react-router'

interface ServiceInfo {
  port: number
  service_type: string
  health: string
  last_seen: string
  title?: string
  pid?: number
}

interface PortsResponse {
  ports: Record<number, ServiceInfo>
  count: number
}

export function PortsDisplay() {
  const [ports, setPorts] = useState<Record<number, ServiceInfo>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchPorts = async () => {
    try {
      const response = await fetch('/v1/ports')
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`)
      }
      const data: PortsResponse = await response.json()
      setPorts(data.ports)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch ports')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchPorts()
    
    // Poll for updates every 2 seconds
    const interval = setInterval(fetchPorts, 2000)
    
    return () => clearInterval(interval)
  }, [])

  const getHealthColor = (health: string) => {
    switch (health) {
      case 'healthy':
        return 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-100'
      case 'unhealthy':
        return 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-100'
      default:
        return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-100'
    }
  }

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

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Loader2 className="h-5 w-5 animate-spin" />
            Detecting Ports...
          </CardTitle>
        </CardHeader>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-red-600">Error</CardTitle>
          <CardDescription>{error}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const portCount = Object.keys(ports).length

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Server className="h-5 w-5" />
          Active Ports
          <Badge variant="secondary">{portCount}</Badge>
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
            {Object.entries(ports).map(([portStr, service]) => (
              <div
                key={portStr}
                className="flex items-center justify-between p-4 border rounded-lg hover:bg-muted/50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  {getServiceIcon(service.service_type)}
                  <div>
                    <div className="font-medium">
                      Port {service.port}
                      {service.title && (
                        <span className="text-sm text-muted-foreground ml-2">
                          • {service.title}
                        </span>
                      )}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {service.service_type.toUpperCase()} service
                      {service.pid && ` • PID ${service.pid}`}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge className={getHealthColor(service.health)}>
                    {service.health}
                  </Badge>
                  {service.service_type === 'http' && service.health === 'healthy' && (
                    <div className="flex gap-1">
                      <Link to="/preview/$port" params={{ port: service.port.toString() }}>
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
                        onClick={() => openService(service.port)}
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