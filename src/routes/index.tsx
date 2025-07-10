import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  StyledDropdown,
  StyledDropdownItem,
} from "@/components/ui/styled-dropdown";
import { Mic, GitBranch, Folder } from "lucide-react";

function Index() {
  const [taskDescription, setTaskDescription] = useState("");
  const [selectedRepo, setSelectedRepo] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("");
  const [selectedSpeed, setSelectedSpeed] = useState("2x");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // TODO: Implement worktree creation and Claude command execution
    console.log(
      "Task:",
      taskDescription,
      "Repo:",
      selectedRepo,
      "Branch:",
      selectedBranch,
      "Speed:",
      selectedSpeed
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
            <StyledDropdown
              value={selectedRepo}
              onValueChange={setSelectedRepo}
              placeholder="vanpelt/grabbit"
              icon={<Folder className="w-4 h-4" />}
            >
              <StyledDropdownItem value="vanpelt/grabbit">
                vanpelt/grabbit
              </StyledDropdownItem>
              <StyledDropdownItem value="vanpelt/catnip">
                vanpelt/catnip
              </StyledDropdownItem>
              <StyledDropdownItem value="vanpelt/claude-mcp">
                vanpelt/claude-mcp
              </StyledDropdownItem>
            </StyledDropdown>

            <StyledDropdown
              value={selectedBranch}
              onValueChange={setSelectedBranch}
              placeholder="main"
              icon={<GitBranch className="w-4 h-4" />}
            >
              <StyledDropdownItem value="main">main</StyledDropdownItem>
              <StyledDropdownItem value="develop">develop</StyledDropdownItem>
              <StyledDropdownItem value="feature/new">feature/new</StyledDropdownItem>
            </StyledDropdown>

            <StyledDropdown
              value={selectedSpeed}
              onValueChange={setSelectedSpeed}
              placeholder="2x"
              icon={<span>⚡</span>}
            >
              <StyledDropdownItem value="1x">1x</StyledDropdownItem>
              <StyledDropdownItem value="2x">2x</StyledDropdownItem>
              <StyledDropdownItem value="4x">4x</StyledDropdownItem>
            </StyledDropdown>
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
