import { useHotkeys } from "react-hotkeys-hook";
import { useState } from "react";

// Centralized keyboard shortcuts configuration
export const KEYBOARD_SHORTCUTS = {
  NEW_WORKSPACE: "cmd+n,ctrl+n",
  // Add more shortcuts here as needed
  // TOGGLE_SIDEBAR: 'cmd+b,ctrl+b',
  // SEARCH: 'cmd+k,ctrl+k',
} as const;

// Hook for managing global keyboard shortcuts
export function useGlobalKeyboardShortcuts() {
  const [newWorkspaceDialogOpen, setNewWorkspaceDialogOpen] = useState(false);

  // New workspace shortcut
  useHotkeys(
    KEYBOARD_SHORTCUTS.NEW_WORKSPACE,
    (event) => {
      event.preventDefault();
      setNewWorkspaceDialogOpen(true);
    },
    {
      enableOnContentEditable: false,
      enableOnFormTags: false,
    },
  );

  return {
    newWorkspaceDialogOpen,
    setNewWorkspaceDialogOpen,
  };
}

// Hook for individual components that need specific shortcuts
export function useKeyboardShortcut(
  shortcut: string,
  callback: (event: KeyboardEvent) => void,
  options?: {
    enableOnContentEditable?: boolean;
    enableOnFormTags?: boolean;
    preventDefault?: boolean;
  },
) {
  useHotkeys(
    shortcut,
    (event) => {
      if (options?.preventDefault !== false) {
        event.preventDefault();
      }
      callback(event);
    },
    {
      enableOnContentEditable: options?.enableOnContentEditable ?? false,
      enableOnFormTags: options?.enableOnFormTags ?? false,
    },
  );
}
