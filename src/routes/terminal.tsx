import { createFileRoute, useSearch } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { useWebSocket } from '@/lib/websocket-context'
import '@xterm/xterm/css/xterm.css'

function TerminalPage() {
  const search = useSearch({ from: '/terminal' })
  const terminalRef = useRef<HTMLDivElement>(null)
  const terminal = useRef<Terminal | null>(null)
  const fitAddon = useRef<FitAddon | null>(null)
  const ws = useRef<WebSocket | null>(null)
  const { setIsConnected } = useWebSocket()

  // Function to send reset command
  const resetTerminal = () => {
    if (ws.current && ws.current.readyState === WebSocket.OPEN) {
      const resetMsg = { type: 'reset' }
      ws.current.send(JSON.stringify(resetMsg))
    }
  }

  // Expose reset function globally for navbar access
  useEffect(() => {
    ;(window as any).resetTerminal = resetTerminal
    return () => {
      delete (window as any).resetTerminal
    }
  }, [])

  // Track search params for reconnection when they change
  const searchKey = JSON.stringify(search)
  
  // Log when search params change for debugging
  useEffect(() => {
    console.log('Search params changed:', search)
  }, [searchKey])

  useEffect(() => {
    // Clear existing connection and terminal when search params change
    if (ws.current) {
      ws.current.close(1000, 'URL changed')
      ws.current = null
      setIsConnected(false)
    }
    
    // Clear terminal content if it exists
    if (terminal.current) {
      terminal.current.clear()
    }

    // Initialize terminal if it doesn't exist
    if (!terminal.current && terminalRef.current) {
      // Create terminal instance
      terminal.current = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: '"FiraCode Nerd Font Mono", "Fira Code", "JetBrains Mono", Monaco, Consolas, monospace',
        theme: {
          background: '#0a0a0a',
          foreground: '#e2e8f0',
          cursor: '#00ff95',
          cursorAccent: '#00ff95',
          selectionBackground: '#333333',
          black: '#0a0a0a',
          red: '#fc8181',
          green: '#68d391',
          yellow: '#f6e05e',
          blue: '#63b3ed',
          magenta: '#d6bcfa',
          cyan: '#4fd1c7',
          white: '#e2e8f0',
          brightBlack: '#4a5568',
          brightRed: '#fc8181',
          brightGreen: '#68d391',
          brightYellow: '#f6e05e',
          brightBlue: '#63b3ed',
          brightMagenta: '#d6bcfa',
          brightCyan: '#4fd1c7',
          brightWhite: '#f7fafc',
        },
        allowProposedApi: true,
        scrollback: 10000,
        altClickMovesCursor: false,
        scrollSensitivity: 1,
        fastScrollSensitivity: 10,
        scrollOnUserInput: false,
        smoothScrollDuration: 0,
        rightClickSelectsWord: false,
        disableStdin: false,
      })

      // Create addons
      fitAddon.current = new FitAddon()
      const webLinksAddon = new WebLinksAddon()

      // Load addons
      terminal.current.loadAddon(fitAddon.current)
      terminal.current.loadAddon(webLinksAddon)
    }

    const terminalInstance = terminal.current
    const fitAddonInstance = fitAddon.current

    // Open terminal in DOM element if not already opened
    if (terminalRef.current && terminalInstance && !terminalInstance.element) {
      terminalInstance.open(terminalRef.current)
      fitAddonInstance?.fit()
      
      // Terminal input handler will be set up after WebSocket connection
    }

    // Set up WebSocket connection
    let reconnectTimeout: number | null = null

    const connectWebSocket = () => {
      // Clean up any existing connection
      if (ws.current) {
        ws.current.close(1000, 'Reconnecting')
        ws.current = null
      }

      // Clear terminal on reconnection
      if (terminal.current) {
        terminal.current.clear()
      }

      // Clear any pending reconnection
      if (reconnectTimeout) {
        clearTimeout(reconnectTimeout)
        reconnectTimeout = null
      }

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const params = new URLSearchParams()
      if (search.agent) {
        params.set('agent', search.agent)
      }
      const wsUrl = `${protocol}//${window.location.host}/v1/pty${params.toString() ? '?' + params.toString() : ''}`
      
      ws.current = new WebSocket(wsUrl)
      ws.current.binaryType = 'arraybuffer'

      ws.current.onopen = () => {
        setIsConnected(true)
        
        // Send terminal size after connection
        if (terminal.current) {
          const resizeMsg = {
            cols: terminal.current.cols,
            rows: terminal.current.rows
          }
          ws.current?.send(JSON.stringify(resizeMsg))
          
          // Focus the terminal for input
          terminal.current.focus()
        }
      }

      ws.current.onmessage = (event) => {
        if (terminal.current) {
          // Handle both binary and text data
          if (event.data instanceof ArrayBuffer) {
            const uint8Array = new Uint8Array(event.data)
            terminal.current.write(uint8Array)
          } else if (typeof event.data === 'string') {
            // Check if this is the shell exit message
            if (event.data.includes('[Shell exited, starting new session...]')) {
              // Clear the terminal before writing the message
              terminal.current.clear()
            }
            terminal.current.write(event.data)
          }
        }
      }

      ws.current.onclose = (event) => {
        setIsConnected(false)
        // Only reconnect if we haven't been cleaned up and it wasn't a normal close
        if (ws.current !== null && event.code !== 1000) {
          reconnectTimeout = window.setTimeout(connectWebSocket, 3000)
        }
      }

      ws.current.onerror = () => {
        setIsConnected(false)
      }
    }

    // Set up terminal input handler
    if (terminalInstance && !terminalInstance._onDataHandlerSet) {
      const onDataHandler = (data: string) => {
        if (ws.current && ws.current.readyState === WebSocket.OPEN) {
          ws.current.send(data)
        }
      }
      
      terminalInstance.onData(onDataHandler)
      terminalInstance._onDataHandlerSet = true
    }

    // Handle window resize
    const handleResize = () => {
      if (fitAddon.current) {
        fitAddon.current.fit()
      }
    }

    window.addEventListener('resize', handleResize)
    
    // Connect WebSocket if terminal is ready
    if (terminalInstance && terminalInstance.element) {
      connectWebSocket()
    }

    // Cleanup function
    return () => {
      window.removeEventListener('resize', handleResize)
      
      // Clear reconnection timeout
      if (reconnectTimeout) {
        clearTimeout(reconnectTimeout)
      }
      
      // Close WebSocket
      if (ws.current) {
        ws.current.close(1000, 'Component unmounting')
        ws.current = null
      }
      
      // Dispose terminal
      if (terminal.current) {
        terminal.current.dispose()
        terminal.current = null
      }
    }
  }, [searchKey]) // Reconnect when search params change

  return (
    <div className="h-screen bg-[#0a0a0a]">
      <div className="h-full bg-[#0a0a0a] overflow-hidden p-4">
        <div 
          className="h-full w-full" 
          ref={terminalRef}
          style={{
            minHeight: '400px',
            minWidth: '600px',
          }}
        />
      </div>
    </div>
  )
}

export const Route = createFileRoute('/terminal')({
  component: TerminalPage,
  validateSearch: (search: Record<string, unknown>) => {
    return {
      agent: (search.agent as string) || undefined,
    }
  },
})