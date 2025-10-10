import React, { forwardRef } from "react";
import {
  TextInput,
  TextInputProps,
  View,
  StyleSheet,
  ViewStyle,
} from "react-native";
import { BlurView } from "expo-blur";
import { theme } from "../../theme";

interface GlassInputProps extends TextInputProps {
  containerStyle?: ViewStyle;
  intensity?: number;
  tint?: "light" | "dark" | "default";
}

export const GlassInput = forwardRef<TextInput, GlassInputProps>(
  (
    { containerStyle, intensity = 20, tint = "default", style, ...props },
    ref,
  ) => {
    return (
      <View style={[styles.container, containerStyle]}>
        <BlurView intensity={intensity} tint={tint} style={styles.blurView}>
          <TextInput
            ref={ref}
            style={[styles.input, style]}
            placeholderTextColor={theme.colors.text.tertiary}
            selectionColor={theme.colors.brand.primary}
            {...props}
          />
        </BlurView>
      </View>
    );
  },
);

GlassInput.displayName = "GlassInput";

const styles = StyleSheet.create({
  container: {
    borderRadius: theme.spacing.radius.control,
    overflow: "hidden",
    ...theme.shadows.sm,
  },
  blurView: {
    borderRadius: theme.spacing.radius.control,
    borderWidth: theme.spacing.borderWidth.thin,
    borderColor: "rgba(255, 255, 255, 0.2)",
    backgroundColor: "rgba(255, 255, 255, 0.1)",
  },
  input: {
    ...theme.typography.body,
    color: theme.colors.text.primary,
    paddingHorizontal: theme.spacing.component.inputPadding,
    paddingVertical: theme.spacing.component.inputPadding,
    minHeight: theme.spacing.hitTarget.minimum,
  },
});

export default GlassInput;
