import ReactDiffViewer, { DiffMethod } from 'react-diff-viewer-continued'

interface InlineDiffProps {
  oldContent: string
  newContent: string
  fileName?: string
}

export function InlineDiff({ oldContent, newContent, fileName }: InlineDiffProps) {
  return (
    <div className="border rounded">
      {fileName && (
        <div className="px-3 py-2 border-b bg-muted/50 text-sm font-mono">
          {fileName}
        </div>
      )}
      <ReactDiffViewer
        oldValue={oldContent}
        newValue={newContent}
        splitView={false}
        compareMethod={DiffMethod.WORDS}
        leftTitle="Before"
        rightTitle="After"
        hideLineNumbers={false}
        styles={{
          diffContainer: {
            fontSize: '12px',
            fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Consolas, "Liberation Mono", Menlo, monospace'
          },
          line: {
            fontSize: '12px'
          }
        }}
      />
    </div>
  )
}