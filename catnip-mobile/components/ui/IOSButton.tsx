import React from "react";
import {
  Pressable,
  Text,
  StyleSheet,
  ActivityIndicator,
  ViewStyle,
  TextStyle,
} from "react-native";
import { theme } from "../../theme";

interface IOSButtonProps {
  title: string;
  onPress: () => void;
  variant?: "primary" | "secondary" | "tertiary" | "destructive";
  size?: "small" | "medium" | "large";
  disabled?: boolean;
  loading?: boolean;
  style?: ViewStyle;
  titleStyle?: TextStyle;
}

export const IOSButton: React.FC<IOSButtonProps> = ({
  title,
  onPress,
  variant = "primary",
  size = "medium",
  disabled = false,
  loading = false,
  style,
  titleStyle,
}) => {
  const getButtonStyle = () => {
    const baseStyle: any[] = [styles.button, styles[size]];

    if (variant === "primary") {
      baseStyle.push(styles.primary);
    } else if (variant === "secondary") {
      baseStyle.push(styles.secondary);
    } else if (variant === "tertiary") {
      baseStyle.push(styles.tertiary);
    } else if (variant === "destructive") {
      baseStyle.push(styles.destructive);
    }

    if (disabled) {
      baseStyle.push(styles.disabled);
    }

    return baseStyle;
  };

  const getTextStyle = () => {
    const baseStyle = [
      styles.text,
      styles[`${size}Text` as keyof typeof styles],
    ];

    if (variant === "primary") {
      baseStyle.push(styles.primaryText);
    } else if (variant === "secondary") {
      baseStyle.push(styles.secondaryText);
    } else if (variant === "tertiary") {
      baseStyle.push(styles.tertiaryText);
    } else if (variant === "destructive") {
      baseStyle.push(styles.destructiveText);
    }

    if (disabled) {
      baseStyle.push(styles.disabledText);
    }

    return baseStyle;
  };

  return (
    <Pressable
      style={({ pressed }) => [
        ...getButtonStyle(),
        pressed && styles.pressed,
        style,
      ]}
      onPress={onPress}
      disabled={disabled || loading}
    >
      {loading ? (
        <ActivityIndicator
          color={variant === "primary" ? "#FFFFFF" : "#007AFF"}
          size="small"
        />
      ) : (
        <Text style={[...getTextStyle(), titleStyle]}>{title}</Text>
      )}
    </Pressable>
  );
};

const styles = StyleSheet.create({
  button: {
    borderRadius: 10,
    alignItems: "center",
    justifyContent: "center",
    flexDirection: "row",
  },

  // Sizes
  small: {
    paddingHorizontal: 16,
    paddingVertical: 8,
    minHeight: 32,
  },
  medium: {
    paddingHorizontal: 20,
    paddingVertical: 12,
    minHeight: 44,
  },
  large: {
    paddingHorizontal: 24,
    paddingVertical: 16,
    minHeight: 50,
  },

  // Variants
  primary: {
    backgroundColor: "#007AFF",
  },
  secondary: {
    backgroundColor: theme.colors.fill.secondary,
    borderWidth: 0.5,
    borderColor: theme.colors.separator.primary,
  },
  tertiary: {
    backgroundColor: "transparent",
  },
  destructive: {
    backgroundColor: "#FF3B30",
  },

  // States
  disabled: {
    opacity: 0.4,
  },
  pressed: {
    opacity: 0.7,
  },

  // Text styles
  text: {
    fontWeight: "600",
    textAlign: "center",
  },
  smallText: {
    fontSize: 14,
  },
  mediumText: {
    fontSize: 16,
  },
  largeText: {
    fontSize: 18,
  },

  // Text variants
  primaryText: {
    color: "#FFFFFF",
  },
  secondaryText: {
    color: "#007AFF",
  },
  tertiaryText: {
    color: "#007AFF",
  },
  destructiveText: {
    color: "#FFFFFF",
  },
  disabledText: {
    opacity: 0.4,
  },
});

export default IOSButton;
