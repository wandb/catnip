import { Stack } from "expo-router";
import { useEffect } from "react";
import { useColorScheme, Platform } from "react-native";
import {
  ThemeProvider,
  DarkTheme,
  DefaultTheme,
} from "@react-navigation/native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import * as SplashScreen from "expo-splash-screen";
import { isLiquidGlassAvailable } from "expo-glass-effect";
import { theme } from "../theme";

// Prevent the splash screen from auto-hiding
SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const colorScheme = useColorScheme();
  const isGlassAvailable = isLiquidGlassAvailable();
  const blurEffect =
    colorScheme === "dark" ? "systemMaterialDark" : "systemMaterialLight";

  useEffect(() => {
    SplashScreen.hideAsync();
  }, []);

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <ThemeProvider value={colorScheme === "dark" ? DarkTheme : DefaultTheme}>
        <Stack
          screenOptions={{
            presentation: "card",
            // @ts-expect-error - headerBackTitleVisible is a valid iOS option
            headerBackTitleVisible: false,
            headerTintColor: "#007AFF",
            headerTitleStyle: {
              fontWeight: "600",
              fontSize: 17,
            },
            contentStyle: {
              backgroundColor: theme.colors.background.grouped,
            },
          }}
        >
          <Stack.Screen name="index" options={{ headerShown: false }} />
          <Stack.Screen name="auth" options={{ headerShown: false }} />
          <Stack.Screen
            name="codespace"
            options={{
              title: "Codespace",
              // @ts-expect-error - headerBackTitleVisible is a valid iOS option
              headerBackTitleVisible: false,
              headerTransparent: true,
              headerShadowVisible: false,
              headerBlurEffect: isGlassAvailable ? undefined : blurEffect,
              headerTitleStyle: {
                fontWeight: "600",
                fontSize: 17,
                color: theme.colors.text.primary,
              },
            }}
          />
          <Stack.Screen
            name="workspaces"
            options={{
              title: "Workspaces",
              headerBackTitle: "",
              // @ts-expect-error - headerBackTitleVisible is a valid iOS option
              headerBackTitleVisible: false,
              headerTransparent: true,
              headerShadowVisible: false,
              headerBlurEffect: isGlassAvailable ? undefined : blurEffect,
              headerTitleStyle: {
                fontWeight: "600",
                fontSize: 17,
                color: theme.colors.text.primary,
              },
            }}
          />
          <Stack.Screen
            name="workspace/[id]"
            options={{
              title: "Workspace",
              headerBackTitle: "",
              // @ts-expect-error - headerBackTitleVisible is a valid iOS option
              headerBackTitleVisible: false,
              headerTransparent: true,
              headerShadowVisible: false,
              headerBlurEffect: isGlassAvailable ? undefined : blurEffect,
              headerTitleStyle: {
                fontWeight: "600",
                fontSize: 17,
                color: theme.colors.text.primary,
              },
            }}
          />
        </Stack>
      </ThemeProvider>
    </GestureHandlerRootView>
  );
}
