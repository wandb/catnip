// iOS 8pt grid system for consistent spacing
export const spacing = {
  // Base unit (8pt grid)
  base: 8,

  // Scale based on 8pt grid
  xs: 4, // 0.5x
  sm: 8, // 1x
  md: 16, // 2x
  lg: 24, // 3x
  xl: 32, // 4x
  xxl: 40, // 5x
  xxxl: 48, // 6x

  // Component-specific spacing
  component: {
    // Padding inside components
    containerPadding: 16,
    cardPadding: 16,
    buttonPadding: 12,
    inputPadding: 12,

    // Margins between components
    sectionSpacing: 24,
    itemSpacing: 12,
    groupSpacing: 8,

    // Header and navigation
    headerPadding: 16,
    navigationBarHeight: 44,
    largeNavigationBarHeight: 96,

    // iOS safe areas and insets
    safeAreaTop: 0, // Handled by SafeAreaView
    safeAreaBottom: 0, // Handled by SafeAreaView
    tabBarHeight: 83, // Standard tab bar height with safe area

    // List and cell spacing
    listItemHeight: 44,
    listItemPadding: 16,
    listSectionSpacing: 35,

    // Form elements
    formFieldSpacing: 16,
    formSectionSpacing: 35,

    // Modal and sheet spacing
    modalPadding: 20,
    sheetPadding: 16,
  },

  // Border radius following iOS design patterns
  radius: {
    none: 0,
    xs: 4,
    sm: 6,
    md: 8,
    lg: 12,
    xl: 16,
    xxl: 20,
    full: 9999,

    // iOS-specific radius values
    button: 8,
    card: 12,
    modal: 12,
    sheet: 10,
    control: 8,
  },

  // Border widths
  borderWidth: {
    none: 0,
    thin: 0.5, // Hairline border
    normal: 1,
    thick: 2,
  },

  // Icon sizes following iOS guidelines
  iconSize: {
    xs: 16,
    sm: 20,
    md: 24,
    lg: 28,
    xl: 32,

    // iOS-specific icon sizes
    tabBar: 30,
    navigation: 22,
    button: 20,
  },

  // Hit targets (minimum 44pt for accessibility)
  hitTarget: {
    minimum: 44,
    button: 44,
    touch: 44,
  },
};

// Helper functions for dynamic spacing
export const getSpacing = (multiplier: number): number => {
  return spacing.base * multiplier;
};

export const getResponsiveSpacing = (
  small: keyof typeof spacing,
  large?: keyof typeof spacing,
): number => {
  // For now, return small spacing - could be enhanced for iPad
  return typeof spacing[small] === "number" ? spacing[small] : spacing.md;
};

export default spacing;
