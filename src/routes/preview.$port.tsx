import { createFileRoute } from "@tanstack/react-router";
import { useState, useEffect, useRef } from "react";
import { Loader2 } from "lucide-react";
import { ErrorDisplay } from "@/components/ErrorDisplay";

export const Route = createFileRoute("/preview/$port")({
  component: PreviewComponent,
});

interface ServiceInfo {
  port: number;
  service_type: string;
  health: string;
  last_seen: string;
  title?: string;
  pid?: number;
}

function PreviewComponent() {
  const { port: portParam } = Route.useParams();
  const port = parseInt(portParam, 10);
  const [service, setService] = useState<ServiceInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [iframeHeight, setIframeHeight] = useState("100vh");
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

  if (loading) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-4rem)]">
        <div className="flex items-center gap-2">
          <Loader2 className="h-5 w-5 animate-spin" />
          <span>Loading service preview...</span>
        </div>
      </div>
    );
  }

  if (error || !service) {
    return (
      <ErrorDisplay
        title="Service Not Found"
        message={
          error ||
          `No service is running on port ${port}. Make sure your application is running and accessible.`
        }
        onRetry={() => window.location.reload()}
        retryLabel="Refresh"
      />
    );
  }

  if (service.service_type !== "http" || service.health !== "healthy") {
    return (
      <ErrorDisplay
        title="Service Not Ready"
        message={`Service on port ${port} is not ready for preview. Service type: ${service.service_type}, Health: ${service.health}`}
        onRetry={() => window.location.reload()}
        retryLabel="Check Again"
      />
    );
  }

  return (
    <div className="h-[calc(100vh-4rem)] relative">
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
        className="w-full border-0"
        style={{
          height: iframeHeight,
          minHeight: "400px",
        }}
        title={`Service preview for port ${port}`}
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox"
      />
    </div>
  );
}
