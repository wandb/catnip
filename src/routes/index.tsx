import { createFileRoute, redirect } from "@tanstack/react-router";
import { shouldShowCodespaceAccess } from "@/lib/utils/codespace-access";

export const Route = createFileRoute("/")({
  beforeLoad: () => {
    // Only redirect to workspace if we're not showing codespace access
    if (!shouldShowCodespaceAccess()) {
      throw redirect({
        to: "/workspace",
      });
    }
  },
});
