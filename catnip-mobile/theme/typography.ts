import { TextStyle } from "react-native";
import { human, systemWeights } from "react-native-typography";

// iOS Dynamic Type scale - follows Human Interface Guidelines
export const textStyles = {
  // Display styles (iOS 17+)
  largeTitle: {
    fontSize: 34,
    fontWeight: "400" as const,
    lineHeight: 41,
    letterSpacing: 0.374,
    fontFamily: "SF Pro Display",
  } as TextStyle,

  title1: {
    fontSize: 28,
    fontWeight: "400" as const,
    lineHeight: 34,
    letterSpacing: 0.364,
    fontFamily: "SF Pro Display",
  } as TextStyle,

  title2: {
    fontSize: 22,
    fontWeight: "400" as const,
    lineHeight: 28,
    letterSpacing: 0.352,
    fontFamily: "SF Pro Display",
  } as TextStyle,

  title3: {
    fontSize: 20,
    fontWeight: "400" as const,
    lineHeight: 25,
    letterSpacing: 0.374,
    fontFamily: "SF Pro Display",
  } as TextStyle,

  // Headline styles
  headline: {
    fontSize: 17,
    fontWeight: "600" as const,
    lineHeight: 22,
    letterSpacing: -0.408,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  // Body styles
  body: {
    fontSize: 17,
    fontWeight: "400" as const,
    lineHeight: 22,
    letterSpacing: -0.408,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  bodyEmphasized: {
    fontSize: 17,
    fontWeight: "600" as const,
    lineHeight: 22,
    letterSpacing: -0.408,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  // Callout
  callout: {
    fontSize: 16,
    fontWeight: "400" as const,
    lineHeight: 21,
    letterSpacing: -0.32,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  calloutEmphasized: {
    fontSize: 16,
    fontWeight: "600" as const,
    lineHeight: 21,
    letterSpacing: -0.32,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  // Subheadline
  subheadline: {
    fontSize: 15,
    fontWeight: "400" as const,
    lineHeight: 20,
    letterSpacing: -0.24,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  subheadlineEmphasized: {
    fontSize: 15,
    fontWeight: "600" as const,
    lineHeight: 20,
    letterSpacing: -0.24,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  // Footnote
  footnote: {
    fontSize: 13,
    fontWeight: "400" as const,
    lineHeight: 18,
    letterSpacing: -0.078,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  footnoteEmphasized: {
    fontSize: 13,
    fontWeight: "600" as const,
    lineHeight: 18,
    letterSpacing: -0.078,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  // Caption styles
  caption1: {
    fontSize: 12,
    fontWeight: "400" as const,
    lineHeight: 16,
    letterSpacing: 0,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  caption1Emphasized: {
    fontSize: 12,
    fontWeight: "600" as const,
    lineHeight: 16,
    letterSpacing: 0,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  caption2: {
    fontSize: 11,
    fontWeight: "400" as const,
    lineHeight: 13,
    letterSpacing: 0.066,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  caption2Emphasized: {
    fontSize: 11,
    fontWeight: "600" as const,
    lineHeight: 13,
    letterSpacing: 0.066,
    fontFamily: "SF Pro Text",
  } as TextStyle,

  // Monospace for code
  codeRegular: {
    fontSize: 13,
    fontWeight: "400" as const,
    lineHeight: 18,
    fontFamily: "SF Mono",
  } as TextStyle,

  codeBold: {
    fontSize: 13,
    fontWeight: "600" as const,
    lineHeight: 18,
    fontFamily: "SF Mono",
  } as TextStyle,
};

// Weight variants for dynamic usage
export const fontWeights = {
  ultraLight: "100" as const,
  thin: "200" as const,
  light: "300" as const,
  regular: "400" as const,
  medium: "500" as const,
  semibold: "600" as const,
  bold: "700" as const,
  heavy: "800" as const,
  black: "900" as const,
};

// Font families
export const fontFamilies = {
  sfProText: "SF Pro Text",
  sfProDisplay: "SF Pro Display",
  sfMono: "SF Mono",
  system: "System",
};

// Helper function to get text style with custom weight
export const getTextStyle = (
  baseStyle: keyof typeof textStyles,
  weight?: keyof typeof fontWeights,
): TextStyle => {
  const style = textStyles[baseStyle];
  if (weight) {
    return {
      ...style,
      fontWeight: fontWeights[weight],
    };
  }
  return style;
};

// Fallback styles using react-native-typography for better cross-platform support
export const fallbackTextStyles = {
  largeTitle: human.largeTitle,
  title1: human.title1,
  title2: human.title2,
  title3: human.title3,
  headline: human.headline,
  body: human.body,
  callout: human.callout,
  subheadline: human.subhead,
  footnote: human.footnote,
  caption1: human.caption1,
  caption2: human.caption2,
};

export default textStyles;
