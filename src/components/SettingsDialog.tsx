"use client";

import * as React from "react";
import { Key, Paintbrush, User, Globe, ExternalLink, Bell } from "lucide-react";
import { wailsApi, isWailsEnvironment, wailsCall } from "@/lib/wails-api";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";

const settingsNav = [
  { name: "Authentication", icon: Key, id: "authentication" },
  { name: "Appearance", icon: Paintbrush, id: "appearance" },
  { name: "Notifications", icon: Bell, id: "notifications" },
  { name: "API", icon: Globe, id: "api" },
];

interface SettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface ClaudeSettings {
  theme: string;
  isAuthenticated: boolean;
  version?: string;
  hasCompletedOnboarding: boolean;
  numStartups: number;
}

interface GitHubAuthStatus {
  status: string;
  error?: string;
  user?: {
    username: string;
    scopes: string[];
  };
}

interface CatnipVersion {
  version: string;
  build: {
    commit: string;
    date: string;
    builtBy: string;
  };
}

// Simple JSON syntax highlighter component
const JsonHighlighter = ({ data }: { data: any }) => {
  const jsonString = JSON.stringify(data, null, 2);

  // Simple regex-based highlighting
  const highlightedJson = jsonString
    .replace(/"([^"]+)":/g, '<span class="text-blue-600">"$1"</span>:')
    .replace(/: "([^"]*)"/g, ': <span class="text-green-600">"$1"</span>')
    .replace(/: (\d+)/g, ': <span class="text-orange-600">$1</span>')
    .replace(
      /: (true|false|null)/g,
      ': <span class="text-purple-600">$1</span>',
    )
    .replace(/\{|\}/g, '<span class="text-gray-600">$&</span>')
    .replace(/\[|\]/g, '<span class="text-gray-600">$&</span>');

  return (
    <pre
      className="p-2 bg-muted rounded text-xs overflow-auto max-h-32 whitespace-pre-wrap"
      dangerouslySetInnerHTML={{ __html: highlightedJson }}
    />
  );
};

export function SettingsDialog({ open, onOpenChange }: SettingsDialogProps) {
  const [activeSection, setActiveSection] = React.useState("authentication");
  const [swaggerData, setSwaggerData] = React.useState<any>(null);
  const [claudeSettings, setClaudeSettings] =
    React.useState<ClaudeSettings | null>(null);
  const [isUpdatingClaudeSettings, setIsUpdatingClaudeSettings] =
    React.useState(false);
  const [githubAuthStatus, setGithubAuthStatus] =
    React.useState<GitHubAuthStatus | null>(null);
  const [catnipVersion, setCatnipVersion] =
    React.useState<CatnipVersion | null>(null);
  const [notificationPermission, setNotificationPermission] =
    React.useState<NotificationPermission>("default");
  const [notificationSupported, setNotificationSupported] =
    React.useState(false);

  // Fetch swagger data when component mounts
  React.useEffect(() => {
    if (activeSection === "api" && !swaggerData) {
      fetch("/swagger/doc.json")
        .then((response) => response.json())
        .then((data) => setSwaggerData(data))
        .catch((error) =>
          console.error("Failed to fetch swagger data:", error),
        );
    }
  }, [activeSection, swaggerData]);

  // Fetch Claude settings when component mounts or when switching to authentication/appearance
  React.useEffect(() => {
    if (
      open &&
      (activeSection === "authentication" || activeSection === "appearance") &&
      !claudeSettings
    ) {
      if (isWailsEnvironment()) {
        wailsCall(() => wailsApi.claude.getSettings())
          .then((data) => setClaudeSettings(data))
          .catch((error) =>
            console.error(
              "Failed to fetch Claude settings from Wails API:",
              error,
            ),
          );
      } else {
        // Fallback to HTTP for development
        fetch("/v1/claude/settings")
          .then((response) => response.json())
          .then((data) => setClaudeSettings(data))
          .catch((error) =>
            console.error("Failed to fetch Claude settings:", error),
          );
      }
    }
  }, [open, activeSection, claudeSettings]);

  // Fetch GitHub auth status when component mounts or when switching to authentication
  React.useEffect(() => {
    if (open && activeSection === "authentication" && !githubAuthStatus) {
      fetch("/v1/auth/github/status")
        .then((response) => response.json())
        .then((data) => setGithubAuthStatus(data))
        .catch((error) =>
          console.error("Failed to fetch GitHub auth status:", error),
        );
    }
  }, [open, activeSection, githubAuthStatus]);

  // Fetch catnip version when component mounts or when switching to authentication
  React.useEffect(() => {
    if (open && activeSection === "authentication" && !catnipVersion) {
      if (isWailsEnvironment()) {
        wailsCall(() => wailsApi.settings.getAppInfo())
          .then((data) => setCatnipVersion(data))
          .catch((error) =>
            console.error(
              "Failed to fetch catnip version from Wails API:",
              error,
            ),
          );
      } else {
        // Fallback to HTTP for development
        fetch("/v1/info")
          .then((response) => response.json())
          .then((data) => setCatnipVersion(data))
          .catch((error) =>
            console.error("Failed to fetch catnip version:", error),
          );
      }
    }
  }, [open, activeSection, catnipVersion]);

  // Check notification support and permission status
  React.useEffect(() => {
    if (open && activeSection === "notifications") {
      const isSupported = "Notification" in window;
      setNotificationSupported(isSupported);
      if (isSupported) {
        setNotificationPermission(Notification.permission);
      }
    }
  }, [open, activeSection]);

  // Function to update Claude theme setting
  const updateClaudeTheme = async (theme: string) => {
    setIsUpdatingClaudeSettings(true);
    try {
      if (isWailsEnvironment()) {
        const updatedSettings = await wailsCall(() =>
          wailsApi.claude.updateSettings({ theme }),
        );
        setClaudeSettings(updatedSettings);
      } else {
        // Fallback to HTTP for development
        const response = await fetch("/v1/claude/settings", {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ theme }),
        });

        if (!response.ok) {
          throw new Error("Failed to update Claude settings");
        }

        const updatedSettings = await response.json();
        setClaudeSettings(updatedSettings);
      }
    } catch (error) {
      console.error("Failed to update Claude settings:", error);
    } finally {
      setIsUpdatingClaudeSettings(false);
    }
  };

  // Function to request notification permission
  const requestNotificationPermission = async () => {
    if (!notificationSupported) {
      console.warn("Notifications are not supported in this browser");
      return;
    }

    try {
      const permission = await Notification.requestPermission();
      setNotificationPermission(permission);

      if (permission === "granted") {
        // Show a test notification
        new Notification("Notifications Enabled", {
          body: "You'll now receive notifications when Claude sessions end.",
          icon: "/logo@2x.webp",
        });
      }
    } catch (error) {
      console.error("Failed to request notification permission:", error);
    }
  };

  // Function to disable notifications (guide user to browser settings)
  const disableNotifications = () => {
    // We can't programmatically disable notifications, so guide the user
    const instructions = window.navigator.userAgent.includes("Chrome")
      ? "Go to Settings > Privacy and security > Site Settings > Notifications, find this site, and select 'Block'"
      : window.navigator.userAgent.includes("Firefox")
        ? "Click the shield icon in the address bar and select 'Block' for notifications"
        : "Check your browser settings to disable notifications for this site";

    alert(`To disable notifications:\n\n${instructions}`);
  };

  // Function to resolve $ref references in swagger spec
  const resolveRef = (ref: string, swaggerData: any): any => {
    if (!ref.startsWith("#/")) return null;
    const path = ref.substring(2).split("/");
    let current = swaggerData;
    for (const segment of path) {
      current = current?.[segment];
      if (!current) return null;
    }
    return current;
  };

  // Function to recursively resolve all $ref in a schema
  const resolveSchema = (
    schema: any,
    swaggerData: any,
    visited = new Set(),
  ): any => {
    if (!schema || typeof schema !== "object") return schema;

    if (schema.$ref) {
      if (visited.has(schema.$ref)) {
        return {
          type: "object",
          description: `Circular reference to ${schema.$ref}`,
        };
      }
      visited.add(schema.$ref);
      const resolved = resolveRef(schema.$ref, swaggerData);
      return resolved ? resolveSchema(resolved, swaggerData, visited) : schema;
    }

    if (Array.isArray(schema)) {
      return schema.map((item) => resolveSchema(item, swaggerData, visited));
    }

    const result: any = {};
    for (const [key, value] of Object.entries(schema)) {
      result[key] = resolveSchema(value, swaggerData, visited);
    }
    return result;
  };

  // Function to generate example data from JSON schema
  const generateExample = (
    schema: any,
    visited = new Set(),
    propName = "",
  ): any => {
    if (!schema || typeof schema !== "object") return null;

    // Handle circular references
    const schemaKey = JSON.stringify(schema);
    if (visited.has(schemaKey)) {
      return "..."; // Indicate circular reference
    }
    visited.add(schemaKey);

    if (schema.example !== undefined) {
      return schema.example;
    }

    switch (schema.type) {
      case "string": {
        if (schema.enum) return schema.enum[0];
        if (schema.format === "date-time") return "2024-01-15T14:30:00Z";
        if (schema.format === "date") return "2024-01-15";
        if (schema.format === "email") return "user@example.com";
        if (schema.format === "uri") return "https://example.com";

        // Smart field name detection
        const fieldName = (schema.title || propName || "").toLowerCase();
        const desc = (schema.description || "").toLowerCase();

        if (
          fieldName.includes("id") ||
          fieldName.includes("uuid") ||
          desc.includes("identifier")
        ) {
          return "abc123-def456-ghi789";
        }
        if (fieldName.includes("name") || desc.includes("name")) {
          return "example-name";
        }
        if (fieldName.includes("email")) return "user@example.com";
        if (fieldName.includes("url") || fieldName.includes("uri"))
          return "https://example.com";
        if (fieldName.includes("branch")) return "main";
        if (fieldName.includes("status")) return "active";
        if (fieldName.includes("message"))
          return "Operation completed successfully";
        if (fieldName.includes("path")) return "/workspace/example-project";
        if (fieldName.includes("title")) return "Example Title";
        if (fieldName.includes("description"))
          return "This is an example description";

        return "example string";
      }

      case "integer":
      case "number":
        if (schema.example !== undefined) return schema.example;
        if (schema.minimum !== undefined) return schema.minimum;
        return schema.type === "integer" ? 42 : 3.14;

      case "boolean":
        return true;

      case "array":
        if (schema.items) {
          const itemExample = generateExample(schema.items, visited, propName);
          return itemExample !== null ? [itemExample] : [];
        }
        return [];

      case "object":
        if (schema.additionalProperties === true) {
          return { key: "value" };
        }
        if (
          schema.additionalProperties &&
          typeof schema.additionalProperties === "object"
        ) {
          const propExample = generateExample(
            schema.additionalProperties,
            visited,
          );
          return { example_key: propExample };
        }
        if (schema.properties) {
          const example: any = {};
          for (const [propName, propSchema] of Object.entries(
            schema.properties,
          )) {
            const propExample = generateExample(
              propSchema as any,
              visited,
              propName,
            );
            if (propExample !== null) {
              example[propName] = propExample;
            }
          }
          return example;
        }
        return {};

      default:
        // Handle allOf, oneOf, anyOf
        if (schema.allOf && schema.allOf.length > 0) {
          let combined = {};
          for (const subSchema of schema.allOf) {
            const subExample = generateExample(subSchema, visited, propName);
            if (subExample && typeof subExample === "object") {
              combined = { ...combined, ...subExample };
            }
          }
          return combined;
        }
        if (schema.oneOf && schema.oneOf.length > 0) {
          return generateExample(schema.oneOf[0], visited, propName);
        }
        if (schema.anyOf && schema.anyOf.length > 0) {
          return generateExample(schema.anyOf[0], visited, propName);
        }
        return null;
    }
  };

  // Function to transform swagger data into endpoints format
  const transformSwaggerEndpoints = (swaggerData: any) => {
    return Object.entries(swaggerData.paths).flatMap(
      ([path, methods]: [string, any]) =>
        Object.entries(methods).map(([method, details]: [string, any]) => ({
          id: `${method}-${path.replace(/[^a-zA-Z0-9]/g, "-")}`,
          method: method.toUpperCase(),
          endpoint: path,
          description:
            details.summary ||
            details.description ||
            `${method.toUpperCase()} ${path}`,
          details: details.description || details.summary || "",
          parameters: details.parameters || [],
          responses: Object.entries(details.responses || {}).map(
            ([status, response]: [string, any]) => ({
              status,
              description: response.description || `HTTP ${status}`,
              schema: response.schema
                ? resolveSchema(response.schema, swaggerData)
                : null,
            }),
          ),
        })),
    );
  };

  const renderSectionContent = () => {
    switch (activeSection) {
      case "authentication":
        return (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-medium mb-4">
                Authentication Status
              </h3>

              <div className="space-y-4">
                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <svg
                      className="h-5 w-5"
                      viewBox="0 0 24 24"
                      fill="currentColor"
                    >
                      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z" />
                    </svg>
                    <div>
                      <p className="font-medium">Catnip</p>
                      <p className="text-sm text-muted-foreground">
                        Container Environment Version
                      </p>
                      {catnipVersion && (
                        <div className="text-xs text-muted-foreground mt-1 space-y-0.5">
                          <div>Version: {catnipVersion.version}</div>
                          {catnipVersion.build.commit &&
                            catnipVersion.build.commit !== "none" && (
                              <div>
                                Commit:{" "}
                                {catnipVersion.build.commit.substring(0, 7)}
                              </div>
                            )}
                          {catnipVersion.build.date &&
                            catnipVersion.build.date !== "unknown" && (
                              <div>Built: {catnipVersion.build.date}</div>
                            )}
                        </div>
                      )}
                    </div>
                  </div>
                  <Badge variant="secondary">
                    {catnipVersion ? catnipVersion.version : "Loading..."}
                  </Badge>
                </div>

                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <User className="h-5 w-5" />
                    <div>
                      <p className="font-medium">Claude</p>
                      <p className="text-sm text-muted-foreground">
                        AI Assistant Authentication
                      </p>
                      {claudeSettings && (
                        <div className="text-xs text-muted-foreground mt-1 space-y-0.5">
                          {claudeSettings.version && (
                            <div>Version: {claudeSettings.version}</div>
                          )}
                          <div>Startups: {claudeSettings.numStartups}</div>
                          <div>
                            Onboarding:{" "}
                            {claudeSettings.hasCompletedOnboarding
                              ? "Complete"
                              : "Incomplete"}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                  <Badge
                    variant={
                      claudeSettings?.isAuthenticated ? "secondary" : "outline"
                    }
                  >
                    {claudeSettings?.isAuthenticated
                      ? "Connected"
                      : "Not Connected"}
                  </Badge>
                </div>

                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <svg
                      className="h-5 w-5"
                      viewBox="0 0 24 24"
                      fill="currentColor"
                    >
                      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                    </svg>
                    <div>
                      <p className="font-medium">GitHub</p>
                      <p className="text-sm text-muted-foreground">
                        Version Control Authentication
                      </p>
                      {githubAuthStatus?.user && (
                        <div className="text-xs text-muted-foreground mt-1 space-y-0.5">
                          <div>User: {githubAuthStatus.user.username}</div>
                          <div>
                            Scopes: {githubAuthStatus.user.scopes.join(", ")}
                          </div>
                        </div>
                      )}
                      {githubAuthStatus?.error && (
                        <div className="text-xs text-red-500 mt-1">
                          {githubAuthStatus.error}
                        </div>
                      )}
                    </div>
                  </div>
                  <Badge
                    variant={
                      githubAuthStatus?.status === "authenticated"
                        ? "secondary"
                        : githubAuthStatus?.status === "error"
                          ? "destructive"
                          : "outline"
                    }
                  >
                    {githubAuthStatus?.status === "authenticated"
                      ? "Connected"
                      : githubAuthStatus?.status === "error"
                        ? "Error"
                        : githubAuthStatus?.status === "pending" ||
                            githubAuthStatus?.status === "waiting"
                          ? "Connecting..."
                          : "Not Connected"}
                  </Badge>
                </div>
              </div>
            </div>
          </div>
        );

      case "appearance":
        return (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-medium mb-4">Appearance Settings</h3>

              <div className="space-y-4">
                <div className="flex items-center justify-between p-4 border rounded-lg">
                  <div>
                    <Label htmlFor="dark-mode" className="font-medium">
                      Dark Mode
                    </Label>
                    <p className="text-sm text-muted-foreground">
                      Toggle between light and dark themes
                    </p>
                  </div>
                  <Switch id="dark-mode" checked={true} />
                </div>

                <div className="p-4 border rounded-lg">
                  <Label className="font-medium mb-2 block">Claude Theme</Label>
                  <p className="text-sm text-muted-foreground mb-3">
                    Choose your preferred Claude interface theme. Theme inherits
                    light/dark mode from global setting.
                  </p>
                  <div className="grid grid-cols-3 gap-2">
                    {["default", "colorblind", "ansi"].map((themeType) => {
                      const isDarkMode = true; // TODO: Get from global theme context
                      const fullTheme = isDarkMode
                        ? themeType === "default"
                          ? "dark"
                          : themeType === "colorblind"
                            ? "dark-daltonized"
                            : "dark-ansi"
                        : themeType === "default"
                          ? "light"
                          : themeType === "colorblind"
                            ? "light-daltonized"
                            : "light-ansi";

                      const isActive = claudeSettings?.theme === fullTheme;

                      return (
                        <Button
                          key={themeType}
                          variant={isActive ? "default" : "outline"}
                          size="sm"
                          disabled={isUpdatingClaudeSettings}
                          onClick={() => updateClaudeTheme(fullTheme)}
                        >
                          {themeType.charAt(0).toUpperCase() +
                            themeType.slice(1)}
                        </Button>
                      );
                    })}
                  </div>
                  {claudeSettings && (
                    <div className="text-xs text-muted-foreground mt-2">
                      Current: {claudeSettings.theme}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        );

      case "notifications":
        return (
          <div className="space-y-6">
            <div>
              <h3 className="text-lg font-medium mb-4">
                Notification Settings
              </h3>
              <p className="text-sm text-muted-foreground mb-6">
                Receive browser notifications when Claude sessions end to stay
                informed about your workflow progress.
              </p>

              <div className="space-y-4">
                {!notificationSupported ? (
                  <div className="p-4 border rounded-lg bg-muted/20">
                    <div className="flex items-center gap-3">
                      <Bell className="h-5 w-5 text-muted-foreground" />
                      <div>
                        <p className="font-medium text-muted-foreground">
                          Notifications Not Supported
                        </p>
                        <p className="text-sm text-muted-foreground">
                          Your browser doesn't support notifications. Please use
                          a modern browser like Chrome, Firefox, or Safari.
                        </p>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center justify-between p-4 border rounded-lg">
                    <div className="flex items-center gap-3">
                      <Bell className="h-5 w-5" />
                      <div>
                        <p className="font-medium">
                          Claude Session Notifications
                        </p>
                        <p className="text-sm text-muted-foreground">
                          Get notified when your Claude sessions end with
                          context about your last todo and branch.
                        </p>
                        <div className="text-xs text-muted-foreground mt-2">
                          {notificationPermission === "granted" && (
                            <span className="text-green-600">
                              ✓ Notifications are enabled
                            </span>
                          )}
                          {notificationPermission === "denied" && (
                            <span className="text-red-600">
                              ✗ Notifications are blocked - check browser
                              settings
                            </span>
                          )}
                          {notificationPermission === "default" && (
                            <span className="text-amber-600">
                              ⚠ Notifications permission not requested
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {notificationPermission === "granted" ? (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={disableNotifications}
                        >
                          Disable
                        </Button>
                      ) : notificationPermission === "denied" ? (
                        <div className="text-right">
                          <Badge
                            variant="outline"
                            className="text-red-600 border-red-300"
                          >
                            Blocked
                          </Badge>
                          <p className="text-xs text-muted-foreground mt-1">
                            Enable in browser settings
                          </p>
                        </div>
                      ) : (
                        <Button
                          variant="default"
                          size="sm"
                          onClick={requestNotificationPermission}
                        >
                          Enable Notifications
                        </Button>
                      )}
                    </div>
                  </div>
                )}

                <div className="p-4 border rounded-lg bg-muted/10">
                  <h4 className="font-medium mb-2 flex items-center gap-2">
                    <span>📋</span>
                    Notification Preview
                  </h4>
                  <p className="text-sm text-muted-foreground mb-3">
                    Here's what notifications will look like:
                  </p>
                  <div className="bg-background border rounded-lg p-3 shadow-sm">
                    <div className="flex items-center gap-3">
                      <img
                        src="/logo@2x.webp"
                        alt="Catnip"
                        className="w-6 h-6 rounded"
                      />
                      <div>
                        <p className="font-medium text-sm">
                          Fix authentication bug (feature/auth-fix)
                        </p>
                        <p className="text-xs text-muted-foreground">
                          Session ended - Last todo: Update password validation
                          logic
                        </p>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="p-4 border rounded-lg bg-blue-50 dark:bg-blue-950/20">
                  <h4 className="font-medium mb-2 flex items-center gap-2 text-blue-700 dark:text-blue-300">
                    <span>ℹ️</span>
                    How It Works
                  </h4>
                  <ul className="text-sm space-y-1 text-blue-600 dark:text-blue-400">
                    <li>• Notifications appear when Claude sessions end</li>
                    <li>• Includes session title and current branch name</li>
                    <li>• Shows your last active todo for context</li>
                    <li>• Only appears when browser tab is in background</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>
        );

      case "api":
        return (
          <div className="space-y-6">
            <div>
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-medium">API Endpoints</h3>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => window.open("/swagger", "_blank")}
                  className="flex items-center gap-2"
                >
                  <ExternalLink className="h-4 w-4" />
                  Swagger UI
                </Button>
              </div>

              {!swaggerData ? (
                <div className="flex items-center justify-center h-96 border rounded-lg">
                  <p className="text-muted-foreground">
                    Loading API documentation...
                  </p>
                </div>
              ) : (
                <ScrollArea className="h-96 border rounded-lg p-4">
                  <Accordion type="single" collapsible className="w-full">
                    {transformSwaggerEndpoints(swaggerData).map((endpoint) => (
                      <AccordionItem key={endpoint.id} value={endpoint.id}>
                        <AccordionTrigger className="hover:no-underline">
                          <div className="flex items-center gap-3 w-full">
                            <Badge
                              variant={
                                endpoint.method === "GET"
                                  ? "secondary"
                                  : endpoint.method === "DELETE"
                                    ? "destructive"
                                    : "default"
                              }
                              className="min-w-[60px] justify-center"
                            >
                              {endpoint.method}
                            </Badge>
                            <code className="font-mono text-sm flex-1 text-left max-w-80 truncate">
                              {endpoint.endpoint}
                            </code>
                            <span className="text-sm text-muted-foreground flex-1 text-left">
                              {endpoint.description}
                            </span>
                          </div>
                        </AccordionTrigger>
                        <AccordionContent className="pt-4">
                          <div className="space-y-4 pl-4">
                            <div>
                              <p className="text-sm text-muted-foreground mb-3">
                                {endpoint.details}
                              </p>
                            </div>

                            {endpoint.parameters.length > 0 && (
                              <div>
                                <h5 className="font-medium mb-2">Parameters</h5>
                                <div className="space-y-2">
                                  {endpoint.parameters.map(
                                    (param: any, idx: number) => (
                                      <div
                                        key={idx}
                                        className="text-sm border rounded p-3 bg-muted/30"
                                      >
                                        <div className="flex items-center gap-2 mb-1">
                                          <code className="font-mono font-medium">
                                            {param.name}
                                          </code>
                                          <Badge
                                            variant="outline"
                                            className="text-xs"
                                          >
                                            {param.in ||
                                              param.type ||
                                              "parameter"}
                                          </Badge>
                                          {param.required && (
                                            <Badge
                                              variant="destructive"
                                              className="text-xs"
                                            >
                                              required
                                            </Badge>
                                          )}
                                        </div>
                                        <p className="text-muted-foreground">
                                          {param.description ||
                                            "No description available"}
                                        </p>
                                      </div>
                                    ),
                                  )}
                                </div>
                              </div>
                            )}

                            <div>
                              <h5 className="font-medium mb-2">Responses</h5>
                              <div className="space-y-2">
                                {endpoint.responses.map(
                                  (response: any, idx: number) => (
                                    <div
                                      key={idx}
                                      className="text-sm border rounded p-3 bg-muted/30"
                                    >
                                      <div className="flex items-center gap-2 mb-1">
                                        <Badge
                                          variant="outline"
                                          className="text-xs"
                                        >
                                          {response.status}
                                        </Badge>
                                        <span className="font-medium">
                                          {response.description}
                                        </span>
                                      </div>
                                      {response.schema && (
                                        <div className="mt-2">
                                          <p className="text-xs text-muted-foreground mb-1">
                                            Example Response:
                                          </p>
                                          <JsonHighlighter
                                            data={generateExample(
                                              response.schema,
                                            )}
                                          />
                                        </div>
                                      )}
                                    </div>
                                  ),
                                )}
                              </div>
                            </div>
                          </div>
                        </AccordionContent>
                      </AccordionItem>
                    ))}
                  </Accordion>
                </ScrollArea>
              )}
            </div>
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0 md:max-h-[600px] md:max-w-[800px] lg:max-w-[1000px]">
        <DialogTitle className="sr-only">Settings</DialogTitle>
        <DialogDescription className="sr-only">
          Customize your settings here.
        </DialogDescription>
        <SidebarProvider className="items-start">
          <Sidebar collapsible="none" className="hidden md:flex">
            <SidebarContent>
              <SidebarGroup>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {settingsNav.map((item) => (
                      <SidebarMenuItem key={item.name}>
                        <SidebarMenuButton
                          asChild
                          isActive={item.id === activeSection}
                        >
                          <button onClick={() => setActiveSection(item.id)}>
                            <item.icon />
                            <span>{item.name}</span>
                          </button>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </SidebarContent>
          </Sidebar>
          <main className="flex h-[580px] flex-1 flex-col overflow-hidden">
            <header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12">
              <div className="flex items-center gap-2 px-4">
                <Breadcrumb>
                  <BreadcrumbList>
                    <BreadcrumbItem className="hidden md:block">
                      <BreadcrumbLink href="#">Settings</BreadcrumbLink>
                    </BreadcrumbItem>
                    <BreadcrumbSeparator className="hidden md:block" />
                    <BreadcrumbItem>
                      <BreadcrumbPage>
                        {
                          settingsNav.find((nav) => nav.id === activeSection)
                            ?.name
                        }
                      </BreadcrumbPage>
                    </BreadcrumbItem>
                  </BreadcrumbList>
                </Breadcrumb>
              </div>
            </header>
            <div className="flex flex-1 flex-col gap-4 overflow-y-auto p-4 pt-0">
              {renderSectionContent()}
            </div>
          </main>
        </SidebarProvider>
      </DialogContent>
    </Dialog>
  );
}
