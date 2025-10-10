import React, { ReactNode } from "react";
import {
  Pressable,
  Text,
  ViewStyle,
  TextStyle,
  StyleSheet,
  View,
  ActivityIndicator,
} from "react-native";
import { BlurView } from "expo-blur";
import * as Haptics from "expo-haptics";
import { theme } from "../../theme";

interface GlassButtonProps {
  children?: ReactNode;
  title?: string;
  onPress?: () => void;
  disabled?: boolean;
  loading?: boolean;
  variant?: "primary" | "secondary" | "tertiary";
  size?: "small" | "medium" | "large";
  style?: ViewStyle;
  textStyle?: TextStyle;
  intensity?: number;
  tint?: "light" | "dark" | "default";
}

export const GlassButton: React.FC<GlassButtonProps> = ({
  children,
  title,
  onPress,
  disabled = false,
  loading = false,
  variant = "primary",
  size = "medium",
  style,
  textStyle,
  intensity = 30,
  tint = "default",
}) => {
  const handlePress = () => {
    if (!disabled && !loading && onPress) {
      Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
      onPress();
    }
  };

  const getButtonStyle = () => {
    switch (variant) {
      case "primary":
        return styles.primaryButton;
      case "secondary":
        return styles.secondaryButton;
      case "tertiary":
        return styles.tertiaryButton;
      default:
        return styles.primaryButton;
    }
  };

  const getTextStyle = () => {
    const baseStyle =
      size === "large" ? theme.typography.headline : theme.typography.callout;

    switch (variant) {
      case "primary":
        return [baseStyle, styles.primaryText];
      case "secondary":
        return [baseStyle, styles.secondaryText];
      case "tertiary":
        return [baseStyle, styles.tertiaryText];
      default:
        return [baseStyle, styles.primaryText];
    }
  };

  const getSizeStyle = () => {
    switch (size) {
      case "small":
        return styles.smallButton;
      case "large":
        return styles.largeButton;
      default:
        return styles.mediumButton;
    }
  };

  return (
    <Pressable
      onPress={handlePress}
      disabled={disabled || loading}
      style={({ pressed }) => [
        styles.container,
        getButtonStyle(),
        getSizeStyle(),
        style,
        (disabled || loading) && styles.disabled,
        pressed && styles.pressed,
      ]}
    >
      <BlurView intensity={intensity} tint={tint} style={styles.blurView}>
        <View style={styles.content}>
          {loading ? (
            <ActivityIndicator color={theme.colors.text.primary} size="small" />
          ) : (
            <>
              {title && (
                <Text style={[getTextStyle(), textStyle]}>{title}</Text>
              )}
              {children}
            </>
          )}
        </View>
      </BlurView>
    </Pressable>
  );
};

const styles = StyleSheet.create({
  container: {
    borderRadius: theme.spacing.radius.button,
    overflow: "hidden",
    ...theme.shadows.sm,
  },
  blurView: {
    flex: 1,
    borderRadius: theme.spacing.radius.button,
    borderWidth: theme.spacing.borderWidth.thin,
    borderColor: "rgba(255, 255, 255, 0.2)",
  },
  content: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.sm,
  },

  // Button variants
  primaryButton: {
    backgroundColor: "rgba(124, 58, 237, 0.3)",
  },
  secondaryButton: {
    backgroundColor: "rgba(255, 255, 255, 0.1)",
  },
  tertiaryButton: {
    backgroundColor: "transparent",
  },

  // Button sizes
  smallButton: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    minHeight: 36,
  },
  mediumButton: {
    paddingVertical: theme.spacing.md,
    paddingHorizontal: theme.spacing.lg,
    minHeight: theme.spacing.hitTarget.button,
  },
  largeButton: {
    paddingVertical: theme.spacing.lg,
    paddingHorizontal: theme.spacing.xl,
    minHeight: 56,
  },

  // Text styles
  primaryText: {
    color: theme.colors.text.primary,
    fontWeight: "600",
  },
  secondaryText: {
    color: theme.colors.text.secondary,
    fontWeight: "500",
  },
  tertiaryText: {
    color: theme.colors.brand.primary,
    fontWeight: "500",
  },

  // States
  disabled: {
    opacity: theme.opacity.disabled,
  },
  pressed: {
    opacity: theme.opacity.pressed,
    transform: [{ scale: 0.98 }],
  },
});

export default GlassButton;
