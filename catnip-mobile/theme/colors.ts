import { PlatformColor, Platform } from "react-native";

// Helper to safely use PlatformColor only on iOS/Android
const platformColor = (name: string): string =>
  Platform.OS === "ios" || Platform.OS === "android"
    ? (PlatformColor(name) as any as string)
    : `#000000`; // Fallback for web

// iOS System Colors - These automatically adapt to light/dark mode
export const systemColors = {
  // Primary system colors
  systemBlue: platformColor("systemBlue"),
  systemGreen: platformColor("systemGreen"),
  systemIndigo: platformColor("systemIndigo"),
  systemOrange: platformColor("systemOrange"),
  systemPink: platformColor("systemPink"),
  systemPurple: platformColor("systemPurple"),
  systemRed: platformColor("systemRed"),
  systemTeal: platformColor("systemTeal"),
  systemYellow: platformColor("systemYellow"),

  // Neutral colors
  label: platformColor("label"),
  secondaryLabel: platformColor("secondaryLabel"),
  tertiaryLabel: platformColor("tertiaryLabel"),
  quaternaryLabel: platformColor("quaternaryLabel"),

  // System fill colors
  systemFill: platformColor("systemFill"),
  secondarySystemFill: platformColor("secondarySystemFill"),
  tertiarySystemFill: platformColor("tertiarySystemFill"),
  quaternarySystemFill: platformColor("quaternarySystemFill"),

  // Background colors
  systemBackground: platformColor("systemBackground"),
  secondarySystemBackground: platformColor("secondarySystemBackground"),
  tertiarySystemBackground: platformColor("tertiarySystemBackground"),

  // Grouped background colors
  systemGroupedBackground: platformColor("systemGroupedBackground"),
  secondarySystemGroupedBackground: platformColor(
    "secondarySystemGroupedBackground",
  ),
  tertiarySystemGroupedBackground: platformColor(
    "tertiarySystemGroupedBackground",
  ),

  // Separator colors
  separator: platformColor("separator"),
  opaqueSeparator: platformColor("opaqueSeparator"),

  // Link color
  link: platformColor("link"),

  // Control colors
  placeholderText: platformColor("placeholderText"),
  controlBackground: platformColor("controlBackground"),
};

// Catnip brand colors
export const brandColors = {
  primary: "#7c3aed", // Purple
  primaryLight: "#a855f7",
  primaryDark: "#5b21b6",

  accent: "#3b82f6", // Blue
  accentLight: "#60a5fa",
  accentDark: "#1d4ed8",

  success: "#10b981",
  warning: "#f59e0b",
  error: "#ef4444",
};

// Semantic colors that combine system and brand colors
export const colors = {
  // Text colors - use system colors for better accessibility
  text: {
    primary: systemColors.label,
    secondary: systemColors.secondaryLabel,
    tertiary: systemColors.tertiaryLabel,
    quaternary: systemColors.quaternaryLabel,
    placeholder: systemColors.placeholderText,
    link: systemColors.link,
  },

  // Background colors
  background: {
    primary: systemColors.systemBackground,
    secondary: systemColors.secondarySystemBackground,
    tertiary: systemColors.tertiarySystemBackground,
    grouped: systemColors.systemGroupedBackground,
    secondaryGrouped: systemColors.secondarySystemGroupedBackground,
    tertiaryGrouped: systemColors.tertiarySystemGroupedBackground,
  },

  // Fill colors for components
  fill: {
    primary: systemColors.systemFill,
    secondary: systemColors.secondarySystemFill,
    tertiary: systemColors.tertiarySystemFill,
    quaternary: systemColors.quaternarySystemFill,
  },

  // Separator colors
  separator: {
    primary: systemColors.separator,
    opaque: systemColors.opaqueSeparator,
  },

  // Brand and accent colors
  brand: brandColors,

  // Status colors using system colors when possible
  status: {
    success: systemColors.systemGreen,
    warning: systemColors.systemOrange,
    error: systemColors.systemRed,
    info: systemColors.systemBlue,
  },

  // Control colors
  control: {
    background: systemColors.controlBackground,
    tint: systemColors.systemBlue,
  },
};

export default colors;
