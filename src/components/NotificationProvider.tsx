import { useEffect } from "react";
import { useNotifications } from "@/lib/useNotifications";
import { useAppStore } from "@/stores/appStore";

export function NotificationProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const notifications = useNotifications();
  const setNotifications = useAppStore((state) => state.setNotifications);

  useEffect(() => {
    // Connect the notifications hook to the app store
    setNotifications(notifications);
  }, [notifications, setNotifications]);

  return <>{children}</>;
}
