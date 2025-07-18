import { useState, useEffect } from "react";

/**
 * useHighlight
 *
 * Hook to provide a temporary highlight effect (with fade-out) for UI elements.
 * When `shouldActivate` is true, applies a highlight class for duration seconds,
 * then fades out over fadeDuration seconds.
 * Returns highlight state and the appropriate tw className for the effect.
 */
export function useHighlight(
  shouldActivate: boolean,
  duration: number = 1500,
  fadeDuration: number = 1000,
) {
  const [shouldHighlight, setShouldHighlight] = useState(shouldActivate);
  const [isFadingOut, setIsFadingOut] = useState(false);

  useEffect(() => {
    if (shouldActivate) {
      setShouldHighlight(true);
      // Start fade out after 1.5 seconds
      const fadeTimer = setTimeout(() => {
        setIsFadingOut(true);
      }, duration);

      // Remove highlight completely after fade completes
      const removeTimer = setTimeout(() => {
        setShouldHighlight(false);
        setIsFadingOut(false);
      }, duration + fadeDuration);

      return () => {
        clearTimeout(fadeTimer);
        clearTimeout(removeTimer);
      };
    }
  }, [shouldActivate]);

  const getHighlightClassName = () => {
    if (!shouldHighlight) return "";
    if (isFadingOut) {
      return "ring-0 border-border transition-all duration-1000 ease-out";
    }
    return "animate-pulse ring-2 ring-blue-300 border-blue-200";
  };

  return {
    shouldHighlight,
    isFadingOut,
    highlightClassName: getHighlightClassName(),
  };
}
