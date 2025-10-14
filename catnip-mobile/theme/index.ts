import colors from "./colors";
import textStyles from "./typography";
import spacing from "./spacing";

// Main theme object that combines all design tokens
export const theme = {
  colors,
  typography: textStyles,
  spacing,

  // Shadows following iOS design patterns
  shadows: {
    none: {
      shadowOpacity: 0,
    },
    sm: {
      shadowColor: "#000",
      shadowOffset: {
        width: 0,
        height: 1,
      },
      shadowOpacity: 0.05,
      shadowRadius: 2,
      elevation: 1,
    },
    md: {
      shadowColor: "#000",
      shadowOffset: {
        width: 0,
        height: 2,
      },
      shadowOpacity: 0.1,
      shadowRadius: 4,
      elevation: 2,
    },
    lg: {
      shadowColor: "#000",
      shadowOffset: {
        width: 0,
        height: 4,
      },
      shadowOpacity: 0.15,
      shadowRadius: 8,
      elevation: 4,
    },
    xl: {
      shadowColor: "#000",
      shadowOffset: {
        width: 0,
        height: 8,
      },
      shadowOpacity: 0.2,
      shadowRadius: 16,
      elevation: 8,
    },
  },

  // Animation durations following iOS timing
  animations: {
    fast: 200,
    normal: 300,
    slow: 500,

    // iOS-specific timing curves
    easeInOut: "ease-in-out",
    easeOut: "ease-out",
    easeIn: "ease-in",

    // Spring animations
    spring: {
      tension: 300,
      friction: 20,
    },
  },

  // Opacity values for states
  opacity: {
    disabled: 0.4,
    pressed: 0.7,
    loading: 0.6,
    overlay: 0.8,
  },
};

export { colors, textStyles, spacing };
export default theme;
