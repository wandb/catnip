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

  useEffect(() => {
    const supported = "Notification" in window;
    setIsSupported(supported);

    if (supported) {
      setPermission(Notification.permission);
    }
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

    // Fallback to browser notification
    if (!isSupported) {
      throw new Error("Notifications are not supported in this browser");
    }

    if (permission !== "granted") {
      throw new Error("Notification permission not granted");
    }

    return new Notification(title, options);
  };

  return {
    permission,
    isSupported,
    requestPermission,
    showNotification,
    sendNativeNotification,
    canShowNotifications: isSupported && permission === "granted",
  };
}
