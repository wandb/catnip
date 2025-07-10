import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Mic } from 'lucide-react'

function Index() {
  const [taskDescription, setTaskDescription] = useState('')
  const [selectedRepo, setSelectedRepo] = useState('')
  const [selectedBranch, setSelectedBranch] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    // TODO: Implement worktree creation and Claude command execution
    console.log('Task:', taskDescription, 'Repo:', selectedRepo, 'Branch:', selectedBranch)
  }

  return (
    <div className="container mx-auto px-4 py-16 min-h-screen flex items-center justify-center">
      <div className="w-full max-w-2xl space-y-8">
        <div className="text-center space-y-4">
          <h1 className="text-5xl font-bold">What are we coding next?</h1>
        </div>
        
        <form onSubmit={handleSubmit} className="space-y-6">
          <div className="relative">
            <Input
              type="text"
              placeholder="Describe a task"
              value={taskDescription}
              onChange={(e) => setTaskDescription(e.target.value)}
              className="w-full h-16 text-lg px-6 pr-16 rounded-2xl border-2 focus:ring-2 focus:ring-primary"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute right-4 top-1/2 transform -translate-y-1/2"
            >
              <Mic className="h-5 w-5" />
            </Button>
          </div>
          
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">📁</span>
              <Select value={selectedRepo} onValueChange={setSelectedRepo}>
                <SelectTrigger className="w-48">
                  <SelectValue placeholder="vanpelt/grabbit" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="vanpelt/grabbit">vanpelt/grabbit</SelectItem>
                  <SelectItem value="vanpelt/catnip">vanpelt/catnip</SelectItem>
                  <SelectItem value="vanpelt/claude-mcp">vanpelt/claude-mcp</SelectItem>
                </SelectContent>
              </Select>
            </div>
            
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">🌿</span>
              <Select value={selectedBranch} onValueChange={setSelectedBranch}>
                <SelectTrigger className="w-32">
                  <SelectValue placeholder="main" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="main">main</SelectItem>
                  <SelectItem value="develop">develop</SelectItem>
                  <SelectItem value="feature/new">feature/new</SelectItem>
                </SelectContent>
              </Select>
            </div>
            
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">⚡</span>
              <Select defaultValue="2x">
                <SelectTrigger className="w-20">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1x">1x</SelectItem>
                  <SelectItem value="2x">2x</SelectItem>
                  <SelectItem value="4x">4x</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          
          <Button
            type="submit"
            className="w-full h-12 text-lg rounded-xl"
            disabled={!taskDescription.trim()}
          >
            Start Coding
          </Button>
        </form>
      </div>
    </div>
  )
}

export const Route = createFileRoute('/')({
  component: Index,
})