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
import {
  Check,
  ChevronsUpDown,
  RefreshCw,
} from "lucide-react";
import type { Repository } from "@/lib/git-api";


interface RepoSelectorProps {
  value: string;
  onValueChange: (value: string) => void;
  repositories: Repository[];
  currentRepositories: Record<string, Repository>;
  loading: boolean;
  placeholder?: string;
}

export function RepoSelector({
  value,
  onValueChange,
  repositories,
  currentRepositories,
  loading,
  placeholder = "Select repository or enter URL...",
}: RepoSelectorProps) {
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
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between"
        >
          {value || placeholder}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[600px] p-0" align="start">
        <Command>
          <CommandInput
            placeholder="Search repositories or type URL..."
            value={value}
            onValueChange={onValueChange}
            onKeyDown={handleKeyDown}
          />
          <CommandList>
            <CommandEmpty>
              {value.startsWith("https://github.com/")
                ? "Press Enter to use this URL"
                : "Type a GitHub repository URL"}
            </CommandEmpty>
            {currentRepositories &&
              Object.keys(currentRepositories).length > 0 && (
                <CommandGroup heading="Current Repositories">
                  {Object.values(currentRepositories).map((repo) => (
                    <CommandItem
                      key={repo.id}
                      value={repo.id.startsWith("local/") ? repo.id : repo.url}
                      onSelect={handleSelect}
                    >
                      <Check
                        className={`mr-2 h-4 w-4 ${
                          value === (repo.id.startsWith("local/") ? repo.id : repo.url) ? "opacity-100" : "opacity-0"
                        }`}
                      />
                      <div className="flex-1">
                        <div className="font-medium">{repo.id}</div>
                        <div className="text-sm text-muted-foreground">
                          {repo.id.startsWith("local/") ? "Local repository (mounted)" : repo.url}
                        </div>
                      </div>
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
            {repositories.length > 0 && (
              <CommandGroup heading="Your GitHub Repositories">
                {repositories.map((repo) => (
                  <CommandItem
                    key={repo.id}
                    value={repo.url}
                    onSelect={handleSelect}
                  >
                    <Check
                      className={`mr-2 h-4 w-4 ${
                        value === repo.url ? "opacity-100" : "opacity-0"
                      }`}
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">
                          {repo.id}
                        </span>
                      </div>
                      {repo.description ? (
                        <div className="text-sm text-muted-foreground">
                          {repo.description}
                        </div>
                      ) : (
                        <div className="text-sm text-muted-foreground">
                          {repo.url}
                        </div>
                      )}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {loading && (
              <CommandGroup heading="Loading...">
                <div className="flex items-center gap-2 text-muted-foreground p-2">
                  <RefreshCw className="animate-spin h-4 w-4" />
                  Loading GitHub repositories...
                </div>
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}