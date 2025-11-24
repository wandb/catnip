import { useState, useEffect } from "react";
import { X, Smartphone } from "lucide-react";

export function MobileAppBanner() {
  const [isVisible, setIsVisible] = useState(false);
  const [isDismissed, setIsDismissed] = useState(false);

  useEffect(() => {
    // Check if user has previously dismissed the banner
    const dismissed = localStorage.getItem("catnip-mobile-banner-dismissed");
    if (dismissed) {
      setIsDismissed(true);
      return;
    }

    // Detect if user is on a mobile device
    const isMobile = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);
    if (isMobile) {
      setIsVisible(true);
    }
  }, []);

  const handleDismiss = () => {
    setIsVisible(false);
    setIsDismissed(true);
    localStorage.setItem("catnip-mobile-banner-dismissed", "true");
  };

  if (!isVisible || isDismissed) {
    return null;
  }

  return (
    <div className="fixed top-0 left-0 right-0 z-[100] bg-gradient-to-r from-purple-600 to-blue-600 text-white px-4 py-3 shadow-lg">
      <div className="container mx-auto flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <Smartphone className="w-5 h-5 flex-shrink-0" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium">Get the Catnip mobile app</p>
            <p className="text-xs opacity-90 truncate">
              Code on the go with our iOS app
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <a
            href="https://testflight.apple.com/join/3gSY17tf"
            target="_blank"
            rel="noopener noreferrer"
            className="bg-white text-purple-600 px-3 py-1.5 rounded-md text-sm font-semibold hover:bg-gray-100 transition-colors"
          >
            Install
          </a>
          <button
            onClick={handleDismiss}
            className="p-1 hover:bg-white/20 rounded transition-colors"
            aria-label="Dismiss banner"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
      </div>
    </div>
  );
}
