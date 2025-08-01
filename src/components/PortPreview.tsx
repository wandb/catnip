import { useState, useEffect, useRef } from "react";
import { Loader2, X, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorDisplay } from "@/components/ErrorDisplay";

interface ServiceInfo {
  port: number;
  service_type: string;
  health: string;
  last_seen: string;
  title?: string;
  pid?: number;
  working_dir?: string;
}

interface PortPreviewProps {
  port: number;
  onClose: () => void;
}

export function PortPreview({ port, onClose }: PortPreviewProps) {
  const [service, setService] = useState<ServiceInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [_iframeHeight, setIframeHeight] = useState("100%");
  const [iframeLoading, setIframeLoading] = useState(true);
  const iframeRef = useRef<HTMLIFrameElement>(null);

  // Fetch service info
  useEffect(() => {
    const fetchService = async () => {
      try {
        const response = await fetch(`/v1/ports/${port}`);
        if (!response.ok) {
          throw new Error(`Port ${port} not found or not active`);
        }
        const data: ServiceInfo = await response.json();
        setService(data);
        setError(null);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Failed to fetch service info",
        );
        setService(null);
      } finally {
        setLoading(false);
      }
    };

    void fetchService();
    // Poll for updates every 5 seconds
    const interval = setInterval(fetchService, 5000);
    return () => clearInterval(interval);
  }, [port]);

  // Handle iframe height messages
  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      // Only accept messages from our iframe origin
      const iframeOrigin = `${window.location.protocol}//${window.location.host}`;
      if (event.origin !== iframeOrigin) {
        return;
      }

      // Handle iframe height updates
      if (event.data?.type === "catnip-iframe-height" && event.data?.height) {
        const height = Math.max(400, event.data.height); // Minimum height
        setIframeHeight(`${height}px`);
      }
    };

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);

  // Handle iframe load
  const handleIframeLoad = () => {
    setIframeLoading(false);
    // Send initial setup message to iframe
    if (iframeRef.current?.contentWindow) {
      const message = {
        type: "catnip-iframe-setup",
        parentOrigin: window.location.origin,
      };
      iframeRef.current.contentWindow.postMessage(message, "*");
    }
  };

  // Handle iframe errors
  const handleIframeError = () => {
    setIframeLoading(false);
    setError(
      `Failed to load service on port ${port}. The service might be down or not responding.`,
    );
  };

  // Open port in new window
  const openInNewWindow = () => {
    window.open(`/${port}/`, "_blank");
  };

  if (loading) {
    return (
      <div className="h-full flex flex-col">
        <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">Port Preview: {port}</span>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={onClose}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
        <div className="flex-1 flex items-center justify-center">
          <div className="flex items-center gap-2">
            <Loader2 className="h-5 w-5 animate-spin" />
            <span>Loading service preview...</span>
          </div>
        </div>
      </div>
    );
  }

  if (error || !service) {
    return (
      <div className="h-full flex flex-col">
        <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">Port Preview: {port}</span>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={onClose}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
        <div className="flex-1">
          <ErrorDisplay
            title="Service Not Found"
            message={
              error ||
              `No service is running on port ${port}. Make sure your application is running and accessible.`
            }
            onRetry={() => window.location.reload()}
            retryLabel="Refresh"
          />
        </div>
      </div>
    );
  }

  if (service.service_type !== "http" || service.health !== "healthy") {
    return (
      <div className="h-full flex flex-col">
        <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">Port Preview: {port}</span>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={onClose}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
        <div className="flex-1">
          <ErrorDisplay
            title="Service Not Ready"
            message={`Service on port ${port} is not ready for preview. Service type: ${service.service_type}, Health: ${service.health}`}
            onRetry={() => window.location.reload()}
            retryLabel="Check Again"
          />
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      {/* Header with title and controls */}
      <div className="px-4 py-2 border-b bg-background/50 backdrop-blur-sm">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">
              {service.title || `Port ${port}`}
            </span>
            <span className="text-xs text-muted-foreground">
              ({service.service_type})
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={openInNewWindow}
              title="Open in new window"
            >
              <ExternalLink className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={onClose}
              title="Close preview"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>

      {/* Iframe container */}
      <div className="flex-1 relative">
        {iframeLoading && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/80 backdrop-blur-sm z-10">
            <div className="flex items-center gap-2">
              <Loader2 className="h-5 w-5 animate-spin" />
              <span>Loading preview...</span>
            </div>
          </div>
        )}
        <iframe
          ref={iframeRef}
          src={`/${port}/`}
          onLoad={handleIframeLoad}
          onError={handleIframeError}
          className="w-full h-full border-0"
          title={`Service preview for port ${port}`}
          sandbox="allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox"
        />
      </div>
    </div>
  );
}
