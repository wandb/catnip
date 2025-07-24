import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Check, ChevronsUpDown, RefreshCw, GitBranch } from "lucide-react";
import type { LocalRepository } from "@/lib/git-api";

interface BranchSelectorProps {
  value: string;
  onValueChange: (value: string) => void;
  branches: string[];
  currentBranch?: string;
  defaultBranch?: string;
  loading: boolean;
  disabled: boolean;
  placeholder?: string;
  isLocalRepo?: boolean;
}

export function BranchSelector({
  value,
  onValueChange,
  branches,
  currentBranch,
  defaultBranch,
  loading,
  disabled,
  placeholder = "Select branch...",
  isLocalRepo = false,
}: BranchSelectorProps) {
  const [open, setOpen] = useState(false);

  const handleSelect = (selectedValue: string) => {
    onValueChange(selectedValue);
    setOpen(false);
  };

  // Sort branches with current branch first (if it exists), then default branch, then others
  const sortedBranches = [...branches].sort((a, b) => {
    if (isLocalRepo && currentBranch) {
      if (a === currentBranch) return -1;
      if (b === currentBranch) return 1;
    }
    if (defaultBranch) {
      if (a === defaultBranch) return isLocalRepo && currentBranch ? 1 : -1;
      if (b === defaultBranch) return isLocalRepo && currentBranch ? -1 : 1;
    }
    return a.localeCompare(b);
  });

  const getBranchBadge = (branch: string) => {
    if (isLocalRepo && branch === currentBranch) {
      return <Badge variant="outline" className="text-xs ml-2">current</Badge>;
    }
    if (branch === defaultBranch) {
      return <Badge variant="secondary" className="text-xs ml-2">default</Badge>;
    }
    return null;
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between"
          disabled={disabled}
        >
          <div className="flex items-center gap-2">
            <GitBranch className="h-4 w-4" />
            {value || placeholder}
          </div>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[400px] p-0" align="start">
        <Command>
          <CommandInput
            placeholder="Search branches..."
            value={value}
            onValueChange={onValueChange}
          />
          <CommandList>
            <CommandEmpty>
              {loading ? "Loading branches..." : "No branches found"}
            </CommandEmpty>
            {loading ? (
              <CommandGroup>
                <div className="flex items-center gap-2 text-muted-foreground p-2">
                  <RefreshCw className="animate-spin h-4 w-4" />
                  Loading branches...
                </div>
              </CommandGroup>
            ) : (
              sortedBranches.length > 0 && (
                <CommandGroup heading="Available Branches">
                  {sortedBranches.map((branch) => (
                    <CommandItem
                      key={branch}
                      value={branch}
                      onSelect={handleSelect}
                    >
                      <Check
                        className={`mr-2 h-4 w-4 ${
                          value === branch ? "opacity-100" : "opacity-0"
                        }`}
                      />
                      <div className="flex-1 flex items-center justify-between">
                        <span className="font-medium">{branch}</span>
                        {getBranchBadge(branch)}
                      </div>
                    </CommandItem>
                  ))}
                </CommandGroup>
              )
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}