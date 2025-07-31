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
import { Check, ChevronsUpDown, RefreshCw } from "lucide-react";
import type { LocalRepository, Repository } from "@/lib/git-api";

interface RepoSelectorProps {
  value: string;
  onValueChange: (value: string) => void;
  repositories: Repository[];
  currentRepositories: Record<string, LocalRepository>;
  loading: boolean;
  placeholder?: string;
  autoExpand?: boolean;
}

export function RepoSelector({
  value,
  onValueChange,
  repositories,
  currentRepositories,
  loading,
  placeholder = "Select repository or enter URL...",
  autoExpand: _autoExpand = false,
}: RepoSelectorProps) {
  const [open, setOpen] = useState(false);
  const [searchValue, setSearchValue] = useState("");

  const handleSelect = (selectedValue: string) => {
    onValueChange(selectedValue);
    setSearchValue(""); // Reset search when selecting
    setOpen(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && searchValue) {
      onValueChange(searchValue);
      setSearchValue("");
      setOpen(false);
    }
  };

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen);
    if (newOpen) {
      // Reset search when opening the combobox
      setSearchValue("");
    }
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
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
            value={searchValue}
            onValueChange={setSearchValue}
            onKeyDown={handleKeyDown}
          />
          <CommandList>
            <CommandEmpty>
              {searchValue.startsWith("https://github.com/") ||
              searchValue.includes("/")
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
                          value ===
                          (repo.id.startsWith("local/") ? repo.id : repo.url)
                            ? "opacity-100"
                            : "opacity-0"
                        }`}
                      />
                      <div className="flex-1">
                        <div className="font-medium">{repo.id}</div>
                        <div className="text-sm text-muted-foreground">
                          {repo.id.startsWith("local/")
                            ? "Local repository (mounted)"
                            : repo.url}
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
                    key={repo.name}
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
                          {repo.fullName ?? repo.name}
                        </span>
                        {repo.private && (
                          <Badge variant="secondary" className="text-xs">
                            Private
                          </Badge>
                        )}
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
