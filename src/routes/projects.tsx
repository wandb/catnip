import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { RepoSelector } from "@/components/RepoSelector";
import { ErrorAlert } from "@/components/ErrorAlert";
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
  FolderOpen,
  Plus,
  RefreshCw,
  Loader2,
  Check,
  ChevronsUpDown,
  GitBranch,
  Trash2,
  Play,
} from "lucide-react";
import { toast } from "sonner";
import { gitApi, type LocalRepository } from "@/lib/git-api";
import { useGitState } from "@/hooks/useGitState";

// Project types
interface Project {
  id: string;
  name: string;
  description: string;
  repository: string;
  branch: string;
  created_at: string;
  status: "active" | "inactive" | "error";
}

// Stubbed project API
const projectApi = {
  async getProjects(): Promise<Project[]> {
    // TODO: Implement actual API call
    return [
      {
        id: "1",
        name: "Sample Project",
        description: "A sample project for testing",
        repository: "https://github.com/example/repo",
        branch: "main",
        created_at: new Date().toISOString(),
        status: "active" as const,
      },
    ];
  },

  async createProject(data: {
    name: string;
    description: string;
    repository: string;
    branch: string;
  }): Promise<Project> {
    // TODO: Implement actual API call
    const project: Project = {
      id: Date.now().toString(),
      ...data,
      created_at: new Date().toISOString(),
      status: "active" as const,
    };
    return project;
  },

  async deleteProject(id: string): Promise<void> {
    // TODO: Implement actual API call
    console.log(`Deleting project ${id}`);
  },

  async getBranches(repository: string): Promise<string[]> {
    // TODO: Implement actual API call
    return ["main", "develop", "feature/new-feature", "release/v1.0"];
  },
};

// Branch selector component
interface BranchSelectorProps {
  repository: string;
  value: string;
  onValueChange: (value: string) => void;
  loading?: boolean;
}

function BranchSelector({ repository, value, onValueChange, loading = false }: BranchSelectorProps) {
  const [open, setOpen] = useState(false);
  const [branches, setBranches] = useState<string[]>([]);
  const [fetchingBranches, setFetchingBranches] = useState(false);

  useEffect(() => {
    if (repository && open) {
      setFetchingBranches(true);
      projectApi.getBranches(repository)
        .then(setBranches)
        .catch(err => {
          console.error("Failed to fetch branches:", err);
          setBranches([]);
        })
        .finally(() => setFetchingBranches(false));
    }
  }, [repository, open]);

  const handleSelect = (selectedValue: string) => {
    onValueChange(selectedValue);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between"
          disabled={!repository || loading}
        >
          {value || "Select branch..."}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[400px] p-0">
        <Command>
          <CommandInput placeholder="Search branches..." />
          <CommandList>
            <CommandEmpty>
              {fetchingBranches ? "Loading branches..." : "No branches found"}
            </CommandEmpty>
            {fetchingBranches ? (
              <CommandGroup>
                <div className="flex items-center gap-2 text-muted-foreground p-2">
                  <RefreshCw className="animate-spin h-4 w-4" />
                  Loading branches...
                </div>
              </CommandGroup>
            ) : (
              <CommandGroup>
                {branches.map((branch) => (
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
                    <GitBranch className="mr-2 h-4 w-4" />
                    {branch}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function ProjectsPage() {
  const {
    repositories,
    gitStatus,
    loading: gitLoading,
    fetchRepositories,
    reposLoading,
  } = useGitState();

  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  
  // Form state
  const [formData, setFormData] = useState({
    name: "",
    description: "",
    repository: "",
    branch: "",
  });

  const [errorAlert, setErrorAlert] = useState<{
    open: boolean;
    title: string;
    description: string;
  }>({
    open: false,
    title: "",
    description: "",
  });

  // Load projects on mount
  useEffect(() => {
    loadProjects();
  }, []);

  const loadProjects = async () => {
    setLoading(true);
    try {
      const projectsData = await projectApi.getProjects();
      setProjects(projectsData);
    } catch (error) {
      console.error("Failed to load projects:", error);
      setErrorAlert({
        open: true,
        title: "Load Failed",
        description: "Failed to load projects. Please try again.",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleCreateProject = async () => {
    if (!formData.name || !formData.repository || !formData.branch) {
      setErrorAlert({
        open: true,
        title: "Validation Error",
        description: "Please fill in all required fields.",
      });
      return;
    }

    setCreating(true);
    try {
      const newProject = await projectApi.createProject(formData);
      setProjects(prev => [...prev, newProject]);
      setFormData({
        name: "",
        description: "",
        repository: "",
        branch: "",
      });
      toast.success("Project created successfully");
    } catch (error) {
      console.error("Failed to create project:", error);
      setErrorAlert({
        open: true,
        title: "Creation Failed",
        description: "Failed to create project. Please try again.",
      });
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteProject = async (id: string) => {
    try {
      await projectApi.deleteProject(id);
      setProjects(prev => prev.filter(p => p.id !== id));
      toast.success("Project deleted successfully");
    } catch (error) {
      console.error("Failed to delete project:", error);
      setErrorAlert({
        open: true,
        title: "Delete Failed",
        description: "Failed to delete project. Please try again.",
      });
    }
  };

  const getStatusColor = (status: Project["status"]) => {
    switch (status) {
      case "active":
        return "bg-green-500";
      case "inactive":
        return "bg-gray-500";
      case "error":
        return "bg-red-500";
      default:
        return "bg-gray-500";
    }
  };

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <div className="flex items-center gap-2 mb-6">
        <FolderOpen size={24} />
        <h1 className="text-3xl font-bold">Projects</h1>
      </div>

      {/* Create New Project */}
      <Card>
        <CardHeader>
          <CardTitle>Create New Project</CardTitle>
          <CardDescription>
            Set up a new project from a Git repository
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="project-name">Project Name *</Label>
              <Input
                id="project-name"
                value={formData.name}
                onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value }))}
                placeholder="Enter project name"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="project-description">Description</Label>
              <Input
                id="project-description"
                value={formData.description}
                onChange={(e) => setFormData(prev => ({ ...prev, description: e.target.value }))}
                placeholder="Enter project description"
              />
            </div>
          </div>
          
          <div className="space-y-2">
            <Label htmlFor="repository">Repository *</Label>
            <div className="flex gap-2">
              <div className="flex-1">
                <RepoSelector
                  value={formData.repository}
                  onValueChange={(value) => setFormData(prev => ({ ...prev, repository: value, branch: "" }))}
                  repositories={repositories}
                  currentRepositories={gitStatus.repositories ?? {}}
                  loading={reposLoading}
                  placeholder="Select repository or enter URL..."
                />
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => fetchRepositories()}
                disabled={reposLoading}
                title="Refresh repositories"
              >
                <RefreshCw className={`h-4 w-4 ${reposLoading ? "animate-spin" : ""}`} />
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="branch">Branch *</Label>
            <BranchSelector
              repository={formData.repository}
              value={formData.branch}
              onValueChange={(value) => setFormData(prev => ({ ...prev, branch: value }))}
              loading={creating}
            />
          </div>

          <Button
            onClick={handleCreateProject}
            disabled={creating || !formData.name || !formData.repository || !formData.branch}
            className="w-full"
          >
            {creating ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Creating Project...
              </>
            ) : (
              <>
                <Plus className="mr-2 h-4 w-4" />
                Create Project
              </>
            )}
          </Button>
        </CardContent>
      </Card>

      {/* Projects List */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Your Projects</CardTitle>
              <CardDescription>
                Manage your active development projects
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={loadProjects}
              disabled={loading}
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex justify-center items-center h-32">
              <Loader2 className="animate-spin h-8 w-8" />
            </div>
          ) : projects.length > 0 ? (
            <div className="space-y-4">
              {projects.map((project) => (
                <div
                  key={project.id}
                  className="flex items-center justify-between p-4 border rounded-lg hover:bg-muted/50 transition-colors"
                >
                  <div className="flex items-center gap-4">
                    <div className={`w-3 h-3 rounded-full ${getStatusColor(project.status)}`} />
                    <div>
                      <div className="flex items-center gap-2">
                        <h3 className="font-semibold">{project.name}</h3>
                        <Badge variant="secondary" className="text-xs">
                          {project.status}
                        </Badge>
                      </div>
                      {project.description && (
                        <p className="text-sm text-muted-foreground mt-1">
                          {project.description}
                        </p>
                      )}
                      <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
                        <span>{project.repository}</span>
                        <div className="flex items-center gap-1">
                          <GitBranch size={14} />
                          <span>{project.branch}</span>
                        </div>
                        <span>Created {new Date(project.created_at).toLocaleDateString()}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="outline" size="sm" title="Start project">
                      <Play size={16} />
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleDeleteProject(project.id)}
                      title="Delete project"
                    >
                      <Trash2 size={16} />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-center py-8">
              <FolderOpen size={48} className="mx-auto text-muted-foreground mb-4" />
              <p className="text-muted-foreground">No projects found</p>
              <p className="text-sm text-muted-foreground">Create your first project to get started</p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Error Alert */}
      <ErrorAlert
        open={errorAlert.open}
        onOpenChange={(open) => setErrorAlert(prev => ({ ...prev, open }))}
        title={errorAlert.title}
        description={errorAlert.description}
      />
    </div>
  );
}

export const Route = createFileRoute("/projects")({
  component: ProjectsPage,
});