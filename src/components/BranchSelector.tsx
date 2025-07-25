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

  console.log("BranchSelector props:", {
    branchCount: branches.length,
    branches,
    currentBranch,
    defaultBranch,
    loading,
    disabled,
  });

  const handleSelect = (selectedValue: string) => {
    onValueChange(selectedValue);
    setOpen(false);
  };

  // Sort branches: default branch first, current/mounted branch second (if different), then all others
  const sortedBranches = [...branches].sort((a, b) => {
    // Ensure we have valid branch names
    if (!a || !b) return 0;

    // Default branch always comes first
    if (defaultBranch) {
      if (a === defaultBranch) return -1;
      if (b === defaultBranch) return 1;
    }

    // Current/mounted branch comes second (if it's different from default)
    if (currentBranch && currentBranch !== defaultBranch) {
      if (a === currentBranch) return -1;
      if (b === currentBranch) return 1;
    }

    // All other branches alphabetically
    return a.localeCompare(b);
  });

  console.log("Sorted branches:", sortedBranches);

  const getBranchBadge = (branch: string) => {
    if (
      branch === defaultBranch &&
      (!isLocalRepo || branch !== currentBranch)
    ) {
      return (
        <Badge variant="secondary" className="text-xs ml-2">
          default
        </Badge>
      );
    }
    if (isLocalRepo && branch === currentBranch) {
      return (
        <Badge variant="outline" className="text-xs ml-2">
          {branch === defaultBranch ? "default/mounted" : "mounted"}
        </Badge>
      );
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
