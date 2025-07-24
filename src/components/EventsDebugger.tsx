import { useAppStore } from '../stores/appStore';
import { Badge } from './ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';

export function EventsDebugger() {
  const { 
    sseConnected, 
    sseError, 
    lastEventId,
    containerStatus,
    getActivePorts,
    getDirtyWorkspaces,
    getRunningProcesses
  } = useAppStore();

  const activePorts = getActivePorts();
  const dirtyWorkspaces = getDirtyWorkspaces();
  const runningProcesses = getRunningProcesses();

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 p-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            SSE Connection
            <Badge variant={sseConnected ? 'default' : 'destructive'}>
              {sseConnected ? 'Connected' : 'Disconnected'}
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            <div className="text-sm">
              <span className="font-medium">Container Status:</span>{' '}
              <Badge variant={containerStatus === 'running' ? 'default' : 'secondary'}>
                {containerStatus}
              </Badge>
            </div>
            {sseError && (
              <div className="text-sm text-red-600">
                <span className="font-medium">Error:</span> {sseError}
              </div>
            )}
            {lastEventId && (
              <div className="text-sm text-gray-600">
                <span className="font-medium">Last Event ID:</span> {lastEventId.slice(-8)}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Active Ports ({activePorts.length})</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {activePorts.length === 0 ? (
              <div className="text-sm text-gray-500">No active ports</div>
            ) : (
              activePorts.map((port) => (
                <div key={port.port} className="flex items-center justify-between">
                  <span className="font-mono">{port.port}</span>
                  <Badge variant="outline">
                    {port.service || 'unknown'}
                  </Badge>
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Git Status</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {dirtyWorkspaces.length === 0 ? (
              <div className="text-sm text-gray-500">All workspaces clean</div>
            ) : (
              dirtyWorkspaces.map((workspace) => (
                <div key={workspace.workspace} className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm">{workspace.workspace}</span>
                    <Badge variant="destructive">dirty</Badge>
                  </div>
                  <div className="text-xs text-gray-600">
                    {workspace.dirtyFiles.length} files changed
                  </div>
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Running Processes ({runningProcesses.length})</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {runningProcesses.length === 0 ? (
              <div className="text-sm text-gray-500">No tracked processes</div>
            ) : (
              runningProcesses.map((process) => (
                <div key={process.pid} className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm">PID {process.pid}</span>
                    <Badge variant="outline">running</Badge>
                  </div>
                  <div className="text-xs text-gray-600 truncate">
                    {process.command}
                  </div>
                  {process.workspace && (
                    <div className="text-xs text-gray-500">
                      {process.workspace}
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}