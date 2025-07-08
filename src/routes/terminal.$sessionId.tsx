import { createFileRoute, useSearch, useParams } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { WebglAddon } from '@xterm/addon-webgl'
import { useWebSocket } from '@/lib/websocket-context'
import { FileDropAddon } from '@/lib/file-drop-addon'
import '@xterm/xterm/css/xterm.css'

function TerminalPage() {
  const search = useSearch({ from: '/terminal/$sessionId' })
  const params = useParams({ from: '/terminal/$sessionId' })
  const terminalRef = useRef<HTMLDivElement>(null)
  const terminal = useRef<Terminal | null>(null)
  const fitAddon = useRef<FitAddon | null>(null)
  const ws = useRef<WebSocket | null>(null)
  const { setIsConnected } = useWebSocket()
  const [fontLoaded, setFontLoaded] = useState(false)
  const messageBuffer = useRef<(string | Uint8Array)[]>([])
  const isTerminalReady = useRef(false)

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

  // Font loading detection
  useEffect(() => {
    const checkFont = async () => {
      try {
        if ('fonts' in document) {
          await document.fonts.ready
          
          // Test if the font is actually working by measuring character width
          const testElement = document.createElement('span')
          testElement.style.fontFamily = '"FiraCode Nerd Font Mono", monospace'
          testElement.style.fontSize = '14px'
          testElement.style.visibility = 'hidden'
          testElement.style.position = 'absolute'
          testElement.textContent = '█' // Box character to test
          document.body.appendChild(testElement)
          
          // Wait a bit for font to render
          setTimeout(() => {
            const rect = testElement.getBoundingClientRect()
            const hasProperWidth = rect.width > 0
            document.body.removeChild(testElement)
            setFontLoaded(hasProperWidth)
          }, 100)
        } else {
          // Fallback: assume font is loaded after a delay
          setTimeout(() => setFontLoaded(true), 1000)
        }
      } catch (error) {
        console.warn('Font loading detection failed:', error)
        setFontLoaded(true)
      }
    }
    
    checkFont()
  }, [])

  // HMR detection and cleanup
  useEffect(() => {
    const handleHMR = () => {
      console.log('HMR detected, reinitializing terminal...')
      // Clear terminal and reset state
      if (terminal.current) {
        terminal.current.clear()
        isTerminalReady.current = false
        messageBuffer.current = []
        // Force re-fit after HMR
        setTimeout(() => {
          if (fitAddon.current) {
            fitAddon.current.fit()
          }
        }, 100)
      }
    }

    // Listen for Vite HMR events
    if (import.meta.hot) {
      import.meta.hot.on('vite:beforeUpdate', handleHMR)
      return () => {
        import.meta.hot?.off('vite:beforeUpdate', handleHMR)
      }
    }
  }, [])

  useEffect(() => {
    // Don't initialize terminal until font is loaded
    if (!fontLoaded) return

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

    // Reset state
    isTerminalReady.current = false
    messageBuffer.current = []

    // Initialize terminal if it doesn't exist
    if (!terminal.current && terminalRef.current) {
      // Create terminal instance
      terminal.current = new Terminal({
        cursorBlink: search.agent !== 'claude',
        fontSize: 14,
        fontFamily: '"FiraCode Nerd Font Mono", "Fira Code", "JetBrains Mono", Monaco, Consolas, monospace',
        theme: {
          background: '#0a0a0a',
          foreground: '#e2e8f0',
          cursor: search.agent === 'claude' ? '#0a0a0a' : '#00ff95',
          cursorAccent: search.agent === 'claude' ? '#0a0a0a' : '#00ff95',
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
      
      // Create file drop addon with WebSocket sender
      const sendData = (data: string) => {
        if (ws.current && ws.current.readyState === WebSocket.OPEN) {
          ws.current.send(data)
        }
      }
      const fileDropAddon = new FileDropAddon(sendData)

      // Load addons
      terminal.current.loadAddon(fitAddon.current)
      terminal.current.loadAddon(webLinksAddon)
      terminal.current.loadAddon(fileDropAddon)
      
      // Try to load WebGL addon for better performance
      try {
        const webglAddon = new WebglAddon()
        terminal.current.loadAddon(webglAddon)
        console.log('WebGL addon loaded successfully')
      } catch (error) {
        console.warn('WebGL addon failed to load, falling back to canvas:', error)
      }
    }

    const terminalInstance = terminal.current
    const fitAddonInstance = fitAddon.current

    // Open terminal in DOM element if not already opened
    if (terminalRef.current && terminalInstance && !terminalInstance.element) {
      terminalInstance.open(terminalRef.current)
      
      // Wait a bit for terminal to be fully ready, then fit
      setTimeout(() => {
        fitAddonInstance?.fit()
        isTerminalReady.current = true
        
        // Process any buffered messages
        if (messageBuffer.current.length > 0) {
          messageBuffer.current.forEach(data => {
            if (terminalInstance) {
              terminalInstance.write(data)
            }
          })
          messageBuffer.current = []
        }
      }, 50)
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
      const urlParams = new URLSearchParams()
      
      // Add session parameter if available
      if (params.sessionId) {
        urlParams.set('session', params.sessionId)
      }
      
      // Add agent parameter if available
      if (search.agent) {
        urlParams.set('agent', search.agent)
      }
      
      const wsUrl = `${protocol}//${window.location.host}/v1/pty${urlParams.toString() ? '?' + urlParams.toString() : ''}`
      
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
          let data: string | Uint8Array
          
          // Handle both binary and text data
          if (event.data instanceof ArrayBuffer) {
            data = new Uint8Array(event.data)
          } else if (typeof event.data === 'string') {
            // Check if this is the shell exit message
            if (event.data.includes('[Shell exited, starting new session...]')) {
              // Clear the terminal before writing the message
              terminal.current.clear()
              isTerminalReady.current = true // Reset ready state
            }
            data = event.data
          } else {
            return
          }

          // Buffer messages if terminal isn't ready yet
          if (!isTerminalReady.current) {
            messageBuffer.current.push(data)
          } else {
            terminal.current.write(data)
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
    if (terminalInstance && !(terminalInstance as any)._onDataHandlerSet) {
      const onDataHandler = (data: string) => {
        if (ws.current && ws.current.readyState === WebSocket.OPEN) {
          ws.current.send(data)
        }
      }
      
      terminalInstance.onData(onDataHandler)
      ;(terminalInstance as any)._onDataHandlerSet = true
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
  }, [searchKey, fontLoaded]) // Reconnect when search params change or font loads

  return (
    <div className="h-screen bg-[#0a0a0a]">
      <div className="h-full bg-[#0a0a0a] overflow-hidden p-4">
        {!fontLoaded && (
          <div className="h-full w-full flex items-center justify-center">
            <div className="text-gray-400 text-sm">
              Loading terminal font...
            </div>
          </div>
        )}
        <div 
          className="h-full w-full" 
          ref={terminalRef}
          style={{
            minHeight: '400px',
            minWidth: '600px',
            visibility: fontLoaded ? 'visible' : 'hidden',
          }}
        />
      </div>
    </div>
  )
}

export const Route = createFileRoute('/terminal/$sessionId')({
  component: TerminalPage,
  validateSearch: (search: Record<string, unknown>) => {
    return {
      agent: (search.agent as string) || undefined,
    }
  },
})