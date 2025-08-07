import { cn } from "@/lib/utils";

interface LoadingSpinnerProps {
  className?: string;
  message?: string;
  size?: "sm" | "md" | "lg";
}

export function LoadingSpinner({
  className,
  message = "Loading...",
  size = "md",
}: LoadingSpinnerProps) {
  const sizeClasses = {
    sm: "h-6 w-6",
    md: "h-10 w-10",
    lg: "h-12 w-12",
  };

  const logoSizes = {
    sm: "h-3 w-3",
    md: "h-5 w-5",
    lg: "h-6 w-6",
  };

  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-4",
        className,
      )}
    >
      <div className="relative">
        {/* Outer ring */}
        <div
          className={cn(
            "absolute inset-0 rounded-full border-2 border-muted",
            sizeClasses[size],
          )}
        />

        {/* Animated ring */}
        <div
          className={cn(
            "rounded-full border-2 border-transparent border-t-primary animate-spin",
            sizeClasses[size],
          )}
        />

        {/* Catnip logo in center */}
        <div className="absolute inset-0 flex items-center justify-center">
          <img
            src="/logo@2x.png"
            alt="Catnip"
            className={cn("animate-pulse", logoSizes[size])}
          />
        </div>
      </div>

      {message && (
        <p className="text-sm text-muted-foreground animate-pulse">{message}</p>
      )}
    </div>
  );
}
