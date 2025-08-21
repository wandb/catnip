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
import { Check, ChevronDown, RefreshCw } from "lucide-react";
import type { LocalRepository, Repository } from "@/lib/git-api";

interface PillSelectorOption {
  value: string;
  label: string;
  description?: string;
  badge?: string;
}

interface PillSelectorProps {
  value: string;
  onValueChange: (value: string) => void;
  options: PillSelectorOption[];
  placeholder?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
  className?: string;
}

export function PillSelector({
  value,
  onValueChange,
  options,
  placeholder = "Select...",
  searchPlaceholder = "Search...",
  emptyMessage = "No options found",
  className = "",
}: PillSelectorProps) {
  const [open, setOpen] = useState(false);

  const selectedOption = options.find((option) => option.value === value);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className={`h-8 px-3 text-xs rounded-full ${className}`}
        >
          <span className="truncate">
            {selectedOption ? selectedOption.label : placeholder}
          </span>
          <ChevronDown className="ml-1 h-3 w-3 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[300px] p-0" align="start">
        <Command>
          <CommandInput placeholder={searchPlaceholder} className="h-9" />
          <CommandList className="max-h-[200px]">
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  onSelect={() => {
                    onValueChange(option.value);
                    setOpen(false);
                  }}
                >
                  <Check
                    className={`mr-2 h-4 w-4 ${
                      value === option.value ? "opacity-100" : "opacity-0"
                    }`}
                  />
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{option.label}</span>
                      {option.badge && (
                        <Badge variant="secondary" className="text-xs">
                          {option.badge}
                        </Badge>
                      )}
                    </div>
                    {option.description && (
                      <div className="text-sm text-muted-foreground">
                        {option.description}
                      </div>
                    )}
                  </div>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

// Specialized components for repositories and branches
interface RepoSelectorPillProps {
  value: string;
  onValueChange: (value: string) => void;
  repositories: LocalRepository[];
  githubRepositories?: Repository[];
  loading?: boolean;
}

export function RepoSelectorPill({
  value,
  onValueChange,
  repositories,
  githubRepositories = [],
  loading = false,
}: RepoSelectorPillProps) {
  // Filter out GitHub repositories that are already available locally
  const filteredGitHubRepos = githubRepositories.filter((repo) => {
    const repoName = repo.name;
    const repoFullName = repo.fullName;

    // Check if we have this repo locally
    const hasLocal = repositories.some((localRepo) => {
      // Check by ID (e.g., "local/doc-crawler" vs "doc-crawler")
      if (localRepo.id.endsWith(`/${repoName}`) || localRepo.id === repoName) {
        return true;
      }

      // Check by URL matching
      if (localRepo.url && localRepo.url === repo.url) {
        return true;
      }

      // Check by full name matching (owner/repo)
      if (
        repoFullName &&
        (localRepo.id.endsWith(`/${repoFullName}`) ||
          localRepo.id === repoFullName)
      ) {
        return true;
      }

      return false;
    });

    return !hasLocal;
  });

  const localOptions: PillSelectorOption[] = repositories.map((repo) => ({
    value: repo.id.startsWith("local/") ? repo.id : repo.url || repo.id,
    label: repo.name || repo.id,
    description: repo.id.startsWith("local/")
      ? "Local repository (mounted)"
      : repo.url,
    badge: repo.id.startsWith("local/") ? "Local" : undefined,
  }));

  const githubOptions: PillSelectorOption[] = filteredGitHubRepos.map(
    (repo) => ({
      value: repo.url,
      label: repo.fullName ?? repo.name,
      description: repo.description || repo.url,
      badge: repo.private ? "Private" : undefined,
    }),
  );

  const allOptions = [...localOptions, ...githubOptions];

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          className="h-8 px-3 text-xs rounded-full"
        >
          <span className="truncate">
            {value
              ? allOptions.find((option) => option.value === value)?.label ||
                value
              : "Select repository..."}
          </span>
          <ChevronDown className="ml-1 h-3 w-3 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[400px] p-0" align="start">
        <Command>
          <CommandInput
            placeholder="Search repositories or type URL..."
            className="h-9"
          />
          <CommandList className="max-h-[300px]">
            <CommandEmpty>Type a GitHub repository URL</CommandEmpty>

            {/* Current Repositories */}
            {localOptions.length > 0 && (
              <CommandGroup heading="Current Repositories">
                {localOptions.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    onSelect={() => onValueChange(option.value)}
                  >
                    <Check
                      className={`mr-2 h-4 w-4 ${
                        value === option.value ? "opacity-100" : "opacity-0"
                      }`}
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{option.label}</span>
                        {option.badge && (
                          <Badge variant="secondary" className="text-xs">
                            {option.badge}
                          </Badge>
                        )}
                      </div>
                      {option.description && (
                        <div className="text-sm text-muted-foreground">
                          {option.description}
                        </div>
                      )}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}

            {/* GitHub Repositories */}
            {githubOptions.length > 0 && (
              <CommandGroup heading="Your GitHub Repositories">
                {githubOptions.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    onSelect={() => onValueChange(option.value)}
                  >
                    <Check
                      className={`mr-2 h-4 w-4 ${
                        value === option.value ? "opacity-100" : "opacity-0"
                      }`}
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{option.label}</span>
                        {option.badge && (
                          <Badge variant="secondary" className="text-xs">
                            {option.badge}
                          </Badge>
                        )}
                      </div>
                      {option.description && (
                        <div className="text-sm text-muted-foreground">
                          {option.description}
                        </div>
                      )}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}

            {/* Loading State */}
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

interface BranchSelectorPillProps {
  value: string;
  onValueChange: (value: string) => void;
  branches: string[];
  defaultBranch?: string;
}

export function BranchSelectorPill({
  value,
  onValueChange,
  branches,
  defaultBranch,
}: BranchSelectorPillProps) {
  const options: PillSelectorOption[] = branches.map((branch) => ({
    value: branch,
    label: branch,
    badge: branch === defaultBranch ? "Default" : undefined,
  }));

  return (
    <PillSelector
      value={value}
      onValueChange={onValueChange}
      options={options}
      placeholder="Select branch..."
      searchPlaceholder="Search branches..."
      emptyMessage="No branches found"
    />
  );
}
