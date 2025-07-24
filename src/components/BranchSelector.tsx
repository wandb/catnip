import { useState } from "react";
import { Button } from "@/components/ui/button";
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
  loading: boolean;
  disabled?: boolean;
  placeholder?: string;
}

export function BranchSelector({
  value,
  onValueChange,
  branches,
  loading,
  disabled = false,
  placeholder = "Select branch...",
}: BranchSelectorProps) {
  const [open, setOpen] = useState(false);

  const handleSelect = (selectedValue: string) => {
    onValueChange(selectedValue);
    setOpen(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && value) {
      setOpen(false);
    }
  };

  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between"
          disabled={disabled}
          onKeyDown={handleKeyDown}
        >
          <div className="flex items-center gap-2 min-w-0">
            <GitBranch className="h-4 w-4 shrink-0" />
            <span className="truncate">
              {value || placeholder}
            </span>
          </div>
          {loading ? (
            <RefreshCw className="ml-2 h-4 w-4 shrink-0 animate-spin" />
          ) : (
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-full p-0" align="start">
        <Command>
          <CommandInput placeholder="Search branches..." />
          <CommandEmpty>No branches found.</CommandEmpty>
          <CommandList>
            <CommandGroup>
              {branches.map((branch) => (
                <CommandItem
                  key={branch}
                  value={branch}
                  onSelect={() => handleSelect(branch)}
                >
                  <Check
                    className={`mr-2 h-4 w-4 ${
                      value === branch ? "opacity-100" : "opacity-0"
                    }`}
                  />
                  <GitBranch className="mr-2 h-4 w-4" />
                  {branch}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}