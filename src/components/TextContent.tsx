import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface TextContentProps {
  content: string
}

export function TextContent({ content }: TextContentProps) {
  if (!content.trim()) {
    return null
  }

  return (
    <div className="prose prose-sm dark:prose-invert max-w-none">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          p: ({ children, ...props }) => (
            <p className="mb-3 last:mb-0" {...props}>
              {children}
            </p>
          ),
          ul: ({ children, ...props }) => (
            <ul className="mb-3 last:mb-0 pl-6" {...props}>
              {children}
            </ul>
          ),
          ol: ({ children, ...props }) => (
            <ol className="mb-3 last:mb-0 pl-6" {...props}>
              {children}
            </ol>
          ),
          pre: ({ children, ...props }) => (
            <pre className="bg-muted p-3 rounded-md overflow-x-auto mb-3 last:mb-0" {...props}>
              {children}
            </pre>
          ),
          code: ({ children, className, ...props }) => {
            const match = /language-(\w+)/.exec(className || '')
            const isInline = !match
            return isInline ? (
              <code className="bg-muted px-1 py-0.5 rounded text-sm" {...props}>
                {children}
              </code>
            ) : (
              <code className={className} {...props}>
                {children}
              </code>
            )
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}