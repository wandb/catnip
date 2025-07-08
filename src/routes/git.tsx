import { createFileRoute, Link } from '@tanstack/react-router';
import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { GitBranch, Github, ExternalLink, Copy, RefreshCw, Trash2, Check, ChevronsUpDown } from 'lucide-react';

// Utility function for relative time display
const getRelativeTime = (date: string | Date) => {
  const now = new Date();
  const then = new Date(date);
  const diffMs = now.getTime() - then.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
  return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
};

const getDuration = (startDate: string | Date, endDate: string | Date) => {
  const start = new Date(startDate);
  const end = new Date(endDate);
  const diffMs = end.getTime() - start.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);

  if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''}`;
  if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ${diffMins % 60} minute${diffMins % 60 !== 1 ? 's' : ''}`;
  return `${Math.floor(diffHours / 24)} day${Math.floor(diffHours / 24) !== 1 ? 's' : ''}`;
};

interface GitStatus {
  repository?: {
    id: string;
    url: string;
    path: string;
    default_branch: string;
  };
  repositories?: Record<string, any>;
  active_worktree?: {
    id: string;
    repo_id: string;
    name: string;
    path: string;
    branch: string;
    commit_hash: string;
    is_active: boolean;
    is_dirty: boolean;
  };
  worktree_count?: number;
}

interface Worktree {
  id: string;
  repo_id: string;
  name: string;
  branch: string;
  source_branch: string;
  path: string;
  commit_hash: string;
  commit_count: number;
  is_active: boolean;
  is_dirty: boolean;
}

interface Repository {
  name: string;
  url: string;
  private: boolean;
  description?: string;
  fullName?: string;
}

function GitPage() {
  const [githubUrl, setGithubUrl] = useState('');
  const [gitStatus, setGitStatus] = useState<GitStatus>({});
  const [worktrees, setWorktrees] = useState<Worktree[]>([]);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repoBranches, setRepoBranches] = useState<Record<string, string[]>>({});
  const [claudeSessions, setClaudeSessions] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(false);
  const [reposLoading, setReposLoading] = useState(false);
  const [comboboxOpen, setComboboxOpen] = useState(false);

  const fetchGitStatus = async () => {
    try {
      const response = await fetch('/v1/git/status');
      if (response.ok) {
        const data = await response.json();
        setGitStatus(data);
        
        // Fetch branches for each repository
        if (data.repositories) {
          const branchPromises = Object.keys(data.repositories).map(async (repoId) => {
            try {
              const branchResponse = await fetch(`/v1/git/branches/${encodeURIComponent(repoId)}`);
              if (branchResponse.ok) {
                const branches = await branchResponse.json();
                return { repoId, branches };
              }
            } catch (error) {
              console.error(`Failed to fetch branches for ${repoId}:`, error);
            }
            return { repoId, branches: [] };
          });
          
          const branchResults = await Promise.all(branchPromises);
          const branchMap: Record<string, string[]> = {};
          branchResults.forEach(({ repoId, branches }) => {
            branchMap[repoId] = branches;
          });
          setRepoBranches(branchMap);
        }
      }
    } catch (error) {
      console.error('Failed to fetch git status:', error);
    }
  };

  const fetchWorktrees = async () => {
    try {
      const response = await fetch('/v1/git/worktrees');
      if (response.ok) {
        const data = await response.json();
        setWorktrees(data);
      }
    } catch (error) {
      console.error('Failed to fetch worktrees:', error);
    }
  };

  const fetchClaudeSessions = async () => {
    try {
      const response = await fetch('/v1/claude/sessions');
      if (response.ok) {
        const data = await response.json();
        setClaudeSessions(data || {});
      } else {
        // Don't error on missing Claude data, just set empty object
        setClaudeSessions({});
      }
    } catch (error) {
      console.error('Failed to fetch Claude sessions:', error);
      // Set empty object on error so UI doesn't break
      setClaudeSessions({});
    }
  };

  const fetchRepositories = async () => {
    setReposLoading(true);
    try {
      const response = await fetch('/v1/git/github/repos');
      if (response.ok) {
        const data = await response.json();
        setRepositories(data);
      }
    } catch (error) {
      console.error('Failed to fetch repositories:', error);
    } finally {
      setReposLoading(false);
    }
  };

  const handleCheckout = async (url: string) => {
    setLoading(true);
    try {
      const urlParts = url.replace('https://github.com/', '').split('/');
      if (urlParts.length >= 2) {
        const [org, repo] = urlParts;
        const response = await fetch(`/v1/git/checkout/${org}/${repo}`, {
          method: 'POST',
        });
        if (response.ok) {
          fetchGitStatus();
          fetchWorktrees();
        }
      }
    } catch (error) {
      console.error('Failed to checkout repository:', error);
    } finally {
      setLoading(false);
    }
  };

  const deleteWorktree = async (id: string) => {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}`, {
        method: 'DELETE',
      });
      if (response.ok) {
        fetchWorktrees();
      }
    } catch (error) {
      console.error('Failed to delete worktree:', error);
    }
  };

  const _createWorktree = async (source: string, name: string) => {
    try {
      const response = await fetch('/v1/git/worktrees', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ source, name }),
      });
      if (response.ok) {
        fetchWorktrees();
      }
    } catch (error) {
      console.error('Failed to create worktree:', error);
    }
  };

  const activateWorktree = async (id: string) => {
    try {
      const response = await fetch(`/v1/git/worktrees/${id}/activate`, {
        method: 'POST',
      });
      if (response.ok) {
        fetchGitStatus();
        fetchWorktrees();
      }
    } catch (error) {
      console.error('Failed to activate worktree:', error);
    }
  };

  const copyRemoteCommand = (url: string) => {
    const command = `git remote add origin ${url} && git fetch origin`;
    navigator.clipboard.writeText(command);
  };

  useEffect(() => {
    fetchGitStatus();
    fetchWorktrees();
    fetchRepositories();
    fetchClaudeSessions();
  }, []);

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <div className="flex items-center gap-2 mb-6">
        <GitBranch size={24} />
        <h1 className="text-3xl font-bold">Git Repository Management</h1>
      </div>

      {/* Worktrees Table */}
      <Card>
        <CardHeader>
          <CardTitle>Worktrees</CardTitle>
          <CardDescription>
            Active worktrees and their branch relationships
          </CardDescription>
        </CardHeader>
        <CardContent>
          {worktrees.length > 0 ? (
            <div className="space-y-2">
              {worktrees.map((worktree) => (
                <div
                  key={worktree.id}
                  className="flex items-center justify-between p-3 border rounded-lg"
                >
                  <div className="flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-medium">{worktree.name}</span>
                      <Badge variant="outline">
                        {worktree.repo_id}@{worktree.source_branch || 'unknown'}
                      </Badge>
                      {worktree.is_dirty ? (
                        <Badge variant="destructive">Dirty</Badge>
                      ) : (
                        <Badge variant="secondary" className="text-xs bg-green-100 text-green-800 border-green-200">
                          Clean
                        </Badge>
                      )}
                      {worktree.commit_count > 0 && (
                        <Badge variant="secondary">+{worktree.commit_count} commits</Badge>
                      )}
                      {claudeSessions[worktree.path] && (
                        <>
                          <Badge variant="default" className="text-xs">
                            {claudeSessions[worktree.path].turnCount} turns
                          </Badge>
                          {claudeSessions[worktree.path].isActive && (
                            <Badge variant="default" className="text-xs bg-green-600">
                              Active Session
                            </Badge>
                          )}
                          {claudeSessions[worktree.path].lastCost > 0 && (
                            <Badge variant="secondary" className="text-xs">
                              ${claudeSessions[worktree.path].lastCost.toFixed(4)}
                            </Badge>
                          )}
                        </>
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground space-y-1">
                      <Link
                        to="/terminal/$sessionId"
                        params={{ sessionId: worktree.name }}
                        className="cursor-pointer hover:text-primary underline-offset-4 hover:underline"
                      >
                        {worktree.path}
                      </Link>
                      {claudeSessions[worktree.path] ? (
                        <div className="space-y-1">
                          {claudeSessions[worktree.path].sessionStartTime && !claudeSessions[worktree.path].isActive ? (
                            // Finished session (has start time and is not active)
                            <p>
                              Finished: {getRelativeTime(claudeSessions[worktree.path].sessionEndTime || claudeSessions[worktree.path].sessionStartTime)} • 
                              Lasted: {getDuration(claudeSessions[worktree.path].sessionStartTime, claudeSessions[worktree.path].sessionEndTime || claudeSessions[worktree.path].sessionStartTime)}
                            </p>
                          ) : claudeSessions[worktree.path].sessionStartTime && claudeSessions[worktree.path].isActive ? (
                            // Active session with timing data
                            <p>
                              Running: {getDuration(claudeSessions[worktree.path].sessionStartTime, new Date())}
                            </p>
                          ) : claudeSessions[worktree.path].isActive ? (
                            // Active session without timestamp data
                            <p>Running: recently started</p>
                          ) : (
                            // Completed session without timestamp data
                            <p>Session completed (timing data unavailable)</p>
                          )}
                        </div>
                      ) : (
                        <div className="space-y-1">
                          <p className="text-xs text-muted-foreground">No Claude sessions</p>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <Link
                      to="/terminal/$sessionId"
                      params={{ sessionId: worktree.name }}
                      search={{ agent: 'claude' }}
                    >
                      <Button
                        variant="outline"
                        size="sm"
                        asChild
                      >
                        <span>Vibe</span>
                      </Button>
                    </Link>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        if (worktree.is_dirty || worktree.commit_count > 0) {
                          if (confirm(`Delete worktree "${worktree.name}"? This worktree has ${worktree.is_dirty ? 'uncommitted changes' : ''} ${worktree.is_dirty && worktree.commit_count > 0 ? 'and ' : ''} ${worktree.commit_count > 0 ? worktree.commit_count + ' commits' : ''}. This action cannot be undone.`)) {
                            deleteWorktree(worktree.id);
                          }
                        } else {
                          deleteWorktree(worktree.id);
                        }
                      }}
                      className="text-destructive hover:text-destructive"
                    >
                      <Trash2 size={16} />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-muted-foreground">No worktrees found</p>
          )}
        </CardContent>
      </Card>

      {/* GitHub URL Input */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Checkout Repository</CardTitle>
              <CardDescription>
                Select from your repositories or enter a GitHub URL
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={fetchRepositories}
              disabled={reposLoading}
              title="Refresh GitHub repositories"
            >
              <RefreshCw className={`h-4 w-4 ${reposLoading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-2">
            <div className="flex-1 space-y-2">
              <Label htmlFor="github-url">GitHub Repository URL</Label>
              <Popover open={comboboxOpen} onOpenChange={setComboboxOpen}>
                <PopoverTrigger asChild>
                  <Button
                    variant="outline"
                    role="combobox"
                    aria-expanded={comboboxOpen}
                    className="w-full justify-between"
                  >
                    {githubUrl || "Select repository or enter URL..."}
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-[600px] p-0" align="start">
                  <Command>
                    <CommandInput 
                      placeholder="Search repositories or type URL..." 
                      value={githubUrl}
                      onValueChange={setGithubUrl}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && githubUrl) {
                          setComboboxOpen(false);
                        }
                      }}
                    />
                    <CommandList>
                      <CommandEmpty>
                        {githubUrl.startsWith('https://github.com/') ? 
                          "Press Enter to use this URL" : 
                          "Type a GitHub repository URL"
                        }
                      </CommandEmpty>
                      {gitStatus.repositories && Object.keys(gitStatus.repositories).length > 0 && (
                        <CommandGroup heading="Current Repositories">
                          {Object.values(gitStatus.repositories).map((repo: any) => (
                            <CommandItem
                              key={repo.id}
                              value={repo.url}
                              onSelect={(value) => {
                                setGithubUrl(value);
                                setComboboxOpen(false);
                              }}
                            >
                              <Check
                                className={`mr-2 h-4 w-4 ${
                                  githubUrl === repo.url ? "opacity-100" : "opacity-0"
                                }`}
                              />
                              <div className="flex-1">
                                <div className="font-medium">{repo.id}</div>
                                <div className="text-sm text-muted-foreground">{repo.url}</div>
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
                              onSelect={(value) => {
                                setGithubUrl(value);
                                setComboboxOpen(false);
                              }}
                            >
                              <Check
                                className={`mr-2 h-4 w-4 ${
                                  githubUrl === repo.url ? "opacity-100" : "opacity-0"
                                }`}
                              />
                              <div className="flex-1">
                                <div className="flex items-center gap-2">
                                  <span className="font-medium">{repo.fullName || repo.name}</span>
                                  {repo.private && (
                                    <Badge variant="secondary" className="text-xs">
                                      Private
                                    </Badge>
                                  )}
                                </div>
                                {repo.description ? (
                                  <div className="text-sm text-muted-foreground">{repo.description}</div>
                                ) : (
                                  <div className="text-sm text-muted-foreground">{repo.url}</div>
                                )}
                              </div>
                            </CommandItem>
                          ))}
                        </CommandGroup>
                      )}
                      {reposLoading && (
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
            </div>
            <Button
              onClick={() => handleCheckout(githubUrl)}
              disabled={!githubUrl || loading}
              className="mt-6"
            >
              {loading ? <RefreshCw className="animate-spin" size={16} /> : 'Checkout'}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Current Status */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Current Repository Status</CardTitle>
              <CardDescription>
                Active repository and worktree information
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                fetchGitStatus();
                fetchWorktrees();
                fetchClaudeSessions();
              }}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {gitStatus.repositories && Object.keys(gitStatus.repositories).length > 0 ? (
            <div className="space-y-4">
              {Object.values(gitStatus.repositories).map((repo: any) => (
                <div key={repo.id} className="space-y-2">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold text-base">{repo.id}</h3>
                    {repoBranches[repo.id] && repoBranches[repo.id].length > 0 && (
                      <>
                        {repoBranches[repo.id].map((branch) => (
                          <Badge 
                            key={branch} 
                            variant="secondary" 
                            className="text-xs cursor-pointer hover:bg-secondary/80"
                            onClick={() => window.open(`${repo.url}/tree/${branch}`, '_blank')}
                          >
                            {branch}
                          </Badge>
                        ))}
                      </>
                    )}
                  </div>
                  <div className="mt-2">
                    <div className="inline-flex items-center gap-2 p-2 bg-muted rounded text-sm font-mono">
                      <code className="text-muted-foreground">
                        git remote add catnip {window.location.origin}/{repo.id.split('/')[1]}.git
                      </code>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          const command = `git remote add catnip ${window.location.origin}/${repo.id.split('/')[1]}.git`;
                          navigator.clipboard.writeText(command);
                        }}
                        className="h-6 w-6 p-0"
                      >
                        <Copy size={12} />
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
              <div className="border-t pt-2">
                <p className="text-xs text-muted-foreground">
                  Total repositories: {Object.keys(gitStatus.repositories).length} | 
                  Total worktrees: {gitStatus.worktree_count || 0}
                </p>
              </div>
            </div>
          ) : (
            <p className="text-muted-foreground">No repositories checked out</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export const Route = createFileRoute('/git')({
  component: GitPage,
});