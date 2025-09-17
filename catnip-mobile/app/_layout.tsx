import { Stack } from 'expo-router';
import { useEffect } from 'react';
import { useColorScheme } from 'react-native';
import { ThemeProvider, DarkTheme, DefaultTheme } from '@react-navigation/native';
import * as SplashScreen from 'expo-splash-screen';

// Prevent the splash screen from auto-hiding
SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const colorScheme = useColorScheme();

  useEffect(() => {
    SplashScreen.hideAsync();
  }, []);

  return (
    <ThemeProvider value={colorScheme === 'dark' ? DarkTheme : DefaultTheme}>
      <Stack
        screenOptions={{
          headerStyle: {
            backgroundColor: '#0a0a0a',
          },
          headerTintColor: '#fff',
          contentStyle: {
            backgroundColor: '#0a0a0a',
          },
          headerTitleStyle: {
            fontWeight: '600',
          },
        }}
      >
        <Stack.Screen name="index" options={{ headerShown: false }} />
        <Stack.Screen name="auth" options={{ headerShown: false }} />
        <Stack.Screen name="codespace" options={{ title: 'Connect Codespace' }} />
        <Stack.Screen name="workspaces" options={{ title: 'Workspaces' }} />
        <Stack.Screen name="workspace/[id]" options={{ title: 'Workspace' }} />
      </Stack>
    </ThemeProvider>
  );
}