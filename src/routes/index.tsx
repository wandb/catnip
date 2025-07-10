import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Mic, GitBranch, Folder } from "lucide-react";

function Index() {
  const [taskDescription, setTaskDescription] = useState("");
  const [selectedRepo, setSelectedRepo] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // TODO: Implement worktree creation and Claude command execution
    console.log(
      "Task:",
      taskDescription,
      "Repo:",
      selectedRepo,
      "Branch:",
      selectedBranch
    );
  };

  return (
    <div className="container mx-auto px-4 py-16 min-h-screen flex items-center justify-center">
      <div className="w-full max-w-2xl space-y-8">
        <div className="text-center space-y-4">
          <h1 className="text-3xl font-bold">What are we coding next?</h1>
        </div>

        <form onSubmit={handleSubmit} className="space-y-6">
          <div className="relative">
            <textarea
              placeholder="Describe a task"
              value={taskDescription}
              onChange={(e) => setTaskDescription(e.target.value)}
              className="w-full h-32 text-lg px-6 py-4 pr-16 rounded-3xl border-0 bg-muted/50 resize-none focus:outline-none focus:ring-0 focus:border-0"
            />

            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute right-4 bottom-4"
            >
              <Mic className="h-5 w-5" />
            </Button>
          </div>

          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">📁</span>
              <Select value={selectedRepo} onValueChange={setSelectedRepo}>
                <SelectTrigger className="w-40 h-8 border-0 bg-muted/50 focus:ring-0 focus:outline-none">
                  <SelectValue placeholder="vanpelt/grabbit" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="vanpelt/grabbit">
                    vanpelt/grabbit
                  </SelectItem>
                  <SelectItem value="vanpelt/catnip">vanpelt/catnip</SelectItem>
                  <SelectItem value="vanpelt/claude-mcp">
                    vanpelt/claude-mcp
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">🌿</span>
              <Select value={selectedBranch} onValueChange={setSelectedBranch}>
                <SelectTrigger className="w-24 h-8 border-0 bg-muted/50 focus:ring-0 focus:outline-none">
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
                <SelectTrigger className="w-16 h-8 border-0 bg-muted/50 focus:ring-0 focus:outline-none">
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

          <div className="flex justify-center">
            <Button
              type="submit"
              className="h-12 px-8 text-lg rounded-xl"
              disabled={!taskDescription.trim()}
            >
              Start Coding
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

export const Route = createFileRoute("/")({
  component: Index,
});
