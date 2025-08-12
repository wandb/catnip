import { useState, useEffect } from "react";

export type NotificationPermission = "default" | "granted" | "denied";

interface NotificationPayload {
  title: string;
  body: string;
  subtitle?: string;
}

export function useNotifications() {
  const [permission, setPermission] =
    useState<NotificationPermission>("default");
  const [isSupported, setIsSupported] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(true);

  useEffect(() => {
    const supported = "Notification" in window;
    setIsSupported(supported);

    if (supported) {
      setPermission(Notification.permission);
    }

    // Fetch notifications setting from the API
    fetch("/v1/claude/settings")
      .then((response) => response.json())
      .then((data) => {
        if (data.notificationsEnabled !== undefined) {
          setNotificationsEnabled(data.notificationsEnabled);
        }
      })
      .catch((error) => {
        console.error("Failed to fetch notifications setting:", error);
      });
  }, []);

  const requestPermission = async (): Promise<NotificationPermission> => {
    if (!isSupported) {
      throw new Error("Notifications are not supported in this browser");
    }

    try {
      const result = await Notification.requestPermission();
      setPermission(result);
      return result;
    } catch (error) {
      console.error("Error requesting notification permission:", error);
      throw error;
    }
  };

  // For explicit permission requests (e.g., from settings UI)
  const requestBrowserPermission =
    async (): Promise<NotificationPermission> => {
      if (!isSupported) {
        throw new Error("Notifications are not supported in this browser");
      }

      try {
        const result = await requestPermission();
        if (result === "granted") {
          // Show a test notification
          new Notification("Notifications Enabled", {
            body: "You'll now receive notifications when Claude sessions end.",
            icon: "/favicon.png",
          });
        }
        return result;
      } catch (error) {
        console.error("Failed to request notification permission:", error);
        throw error;
      }
    };

  const sendNativeNotification = async (
    payload: NotificationPayload,
  ): Promise<boolean> => {
    try {
      const response = await fetch("/v1/notifications", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      console.log("Native notification sent successfully");
      return true;
    } catch (error) {
      console.warn("Failed to send native notification:", error);
      return false;
    }
  };

  const showNotification = async (
    title: string,
    options?: NotificationOptions,
  ) => {
    // Check if notifications are enabled in settings
    if (!notificationsEnabled) {
      console.log("Notifications are disabled in settings");
      return null;
    }

    const payload: NotificationPayload = {
      title,
      body: options?.body || "",
      subtitle: options?.tag,
    };

    // Try native notification first (will work when TUI is connected)
    const nativeSent = await sendNativeNotification(payload);

    if (nativeSent) {
      return null; // Native notification was sent
    }

    // Native notification failed, fall back to browser notification
    if (!isSupported) {
      console.warn("Notifications are not supported in this browser");
      return null;
    }

    // If we don't have permission, request it first
    if (permission !== "granted") {
      console.log(
        "Browser notification permission not granted, requesting permission...",
      );
      try {
        const newPermission = await requestPermission();
        if (newPermission !== "granted") {
          console.warn("Browser notification permission denied");
          return null;
        }
      } catch (error) {
        console.error(
          "Failed to request browser notification permission:",
          error,
        );
        return null;
      }
    }

    // Show browser notification
    return new Notification(title, options);
  };

  return {
    permission,
    isSupported,
    notificationsEnabled,
    requestPermission,
    requestBrowserPermission,
    showNotification,
    sendNativeNotification,
    canShowNotifications: notificationsEnabled,
  };
}
