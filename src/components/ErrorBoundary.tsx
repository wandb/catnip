import { Component, ReactNode } from 'react'
import { ErrorDisplay } from './ErrorDisplay'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: any) {
    console.error('ErrorBoundary caught an error:', error, errorInfo)
  }

  handleReset = () => {
    this.setState({ hasError: false, error: null })
    window.location.reload()
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen flex items-center justify-center p-4">
          <ErrorDisplay
            title="Something went wrong"
            message={this.state.error?.message || 'An unexpected error occurred. Please try refreshing the page.'}
            onRetry={this.handleReset}
            retryLabel="Refresh Page"
          />
        </div>
      )
    }

    return this.props.children
  }
}