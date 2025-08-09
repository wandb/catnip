import { useState } from "react";
import { Button } from "@/components/ui/button";
import { GitBranch } from "lucide-react";
import { BranchSelector } from "./BranchSelector";

interface BranchDisplayProps {
  value: string;
  onValueChange: (value: string) => void;
  branches: string[];
  currentBranch?: string;
  defaultBranch?: string;
  loading: boolean;
  disabled?: boolean;
  isLocalRepo?: boolean;
}

export function BranchDisplay({
  value,
  onValueChange,
  branches,
  currentBranch,
  defaultBranch,
  loading,
  disabled = false,
  isLocalRepo = false,
}: BranchDisplayProps) {
  const [showDropdown, setShowDropdown] = useState(false);

  const handleToggleDropdown = () => {
    if (!disabled) {
      setShowDropdown(!showDropdown);
    }
  };

  const handleBranchSelect = (branch: string) => {
    onValueChange(branch);
    setShowDropdown(false);
  };

  if (showDropdown) {
    return (
      <BranchSelector
        value={value}
        onValueChange={handleBranchSelect}
        branches={branches}
        currentBranch={currentBranch}
        defaultBranch={defaultBranch}
        loading={loading}
        disabled={disabled}
        isLocalRepo={isLocalRepo}
      />
    );
  }

  return (
    <Button
      variant="outline"
      onClick={handleToggleDropdown}
      disabled={disabled}
      className="w-full px-3 text-sm font-normal text-muted-foreground hover:text-foreground transition-colors"
    >
      <GitBranch className="h-3 w-3 mr-1 flex-shrink-0" />
      <span className="truncate">{loading ? "loading..." : `@${value}`}</span>
    </Button>
  );
}
