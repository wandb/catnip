/**
 * Check if we should show the codespace access interface
 * This determines whether we're on catnip.run, a subdomain, or the wrangler dev environment
 */
export function shouldShowCodespaceAccess(): boolean {
  if (typeof window === "undefined") return false;

  return (
    (window.location.hostname === "catnip.run" ||
      window.location.hostname.endsWith(".catnip.run") ||
      (window.location.hostname === "localhost" &&
        window.location.port === "8787")) &&
    window.location.pathname === "/"
  );
}
