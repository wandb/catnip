import React, { ReactNode } from "react";
import { View, ViewStyle, StyleSheet } from "react-native";
import { BlurView } from "expo-blur";
import { theme } from "../../theme";

interface GlassCardProps {
  children: ReactNode;
  style?: ViewStyle;
  intensity?: number;
  tint?: "light" | "dark" | "default";
  borderRadius?: number;
  padding?: number;
}

export const GlassCard: React.FC<GlassCardProps> = ({
  children,
  style,
  intensity = 20,
  tint = "default",
  borderRadius = theme.spacing.radius.lg,
  padding = theme.spacing.component.cardPadding,
}) => {
  return (
    <View style={[styles.container, { borderRadius }, style]}>
      <BlurView
        intensity={intensity}
        tint={tint}
        style={[
          styles.blurView,
          {
            borderRadius,
            padding,
          },
        ]}
      >
        <View style={styles.overlay} />
        {children}
      </BlurView>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    overflow: "hidden",
    ...theme.shadows.md,
  },
  blurView: {
    flex: 1,
    backgroundColor: "rgba(255, 255, 255, 0.1)",
    borderWidth: theme.spacing.borderWidth.thin,
    borderColor: "rgba(255, 255, 255, 0.2)",
  },
  overlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "rgba(255, 255, 255, 0.05)",
  },
});

export default GlassCard;
