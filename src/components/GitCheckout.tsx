import { useEffect, useState } from "react";
import { useParams, useNavigate } from "@tanstack/react-router";
import { Loader2, AlertCircle } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";

interface CheckoutResponse {
  repository: {
    id: string;
    default_branch: string;
  };
  worktree: {
    name: string;
    branch: string;
  };
  message: string;
}

export function GitCheckout() {
  const { _splat: path } = useParams({ from: "/gh/$" });
  const navigate = useNavigate({ from: "/gh/$" });
  const [status, setStatus] = useState<"loading" | "error" | "success">(
    "loading",
  );
  const [error, setError] = useState<string>("");
  const [progress, setProgress] = useState<string>("Initializing checkout...");

  useEffect(() => {
    if (!path) {
      setError("Invalid repository path");
      setStatus("error");
      return;
    }

    // Parse path to extract org/repo and optional branch
    // Support both /gh/org/repo@branch and /gh/org/repo?branch=branch formats
    let pathWithoutBranch = path;
    let branchFromPath = "";

    // Check for @branch syntax
    if (path.includes("@")) {
      const atIndex = path.lastIndexOf("@");
      pathWithoutBranch = path.substring(0, atIndex);
      branchFromPath = path.substring(atIndex + 1);
    }

    const pathParts = pathWithoutBranch.split("/");
    if (pathParts.length < 2) {
      setError(
        "Invalid repository format. Expected: owner/repo or owner/repo@branch",
      );
      setStatus("error");
      return;
    }

    const org = pathParts[0];
    const repo = pathParts.slice(1).join("/");

    // Branch from @syntax takes precedence over query param
    const branch =
      branchFromPath ||
      new URLSearchParams(window.location.search).get("branch") ||
      "";

    void performCheckout(org, repo, branch);
  }, [path]);

  const performCheckout = async (org: string, repo: string, branch: string) => {
    try {
      const repoDisplay = branch
        ? `${org}/${repo}@${branch}`
        : `${org}/${repo}`;
      setProgress(`Checking out ${repoDisplay}...`);

      const url = branch
        ? `/v1/git/checkout/${org}/${repo}?branch=${branch}`
        : `/v1/git/checkout/${org}/${repo}`;

      const response = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || "Failed to checkout repository");
      }

      const result = data as CheckoutResponse;
      setStatus("success");
      setProgress("Checkout complete! Redirecting...");

      // Redirect to terminal with the worktree session
      // Format: /terminal/repo/branch?agent=claude
      const repoName = repo.replace("/", "-");
      const branchName =
        result.worktree.branch || result.repository.default_branch;
      const sessionId = `${repoName}/${branchName}`;

      setTimeout(() => {
        void navigate({
          to: "/terminal/$sessionId",
          params: { sessionId },
          search: { agent: "claude" },
        });
      }, 1000);
    } catch (err) {
      console.error("Checkout error:", err);
      setError(
        err instanceof Error ? err.message : "An unexpected error occurred",
      );
      setStatus("error");
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md space-y-6 p-8">
        <div className="text-center">
          <h1 className="text-2xl font-bold">Git Repository Checkout</h1>
          <p className="mt-2 text-muted-foreground">
            {path && `Checking out ${path}`}
          </p>
        </div>

        {status === "loading" && (
          <div className="space-y-4">
            <div className="flex items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
            <p className="text-center text-sm text-muted-foreground">
              {progress}
            </p>
          </div>
        )}

        {status === "error" && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {status === "success" && (
          <div className="space-y-4">
            <div className="flex items-center justify-center">
              <div className="rounded-full bg-green-100 p-3 dark:bg-green-900">
                <svg
                  className="h-6 w-6 text-green-600 dark:text-green-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              </div>
            </div>
            <p className="text-center text-sm text-muted-foreground">
              {progress}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
