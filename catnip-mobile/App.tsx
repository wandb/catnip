import React, { useEffect, useState } from 'react';
import { NavigationContainer } from '@react-navigation/native';
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';
import * as SecureStore from 'expo-secure-store';
import AuthScreen from './src/screens/AuthScreen';
import CodespaceAccessScreen from './src/screens/CodespaceAccessScreen';
import WorkspaceListScreen from './src/screens/WorkspaceListScreen';
import WorkspaceDetailScreen from './src/screens/WorkspaceDetailScreen';
import { AuthProvider } from './src/contexts/AuthContext';
import { ApiProvider } from './src/contexts/ApiContext';

export type RootStackParamList = {
  Auth: undefined;
  CodespaceAccess: undefined;
  WorkspaceList: undefined;
  WorkspaceDetail: {
    workspaceId: string;
    worktreeName: string;
    repository: string;
    branch: string;
  };
};

const Stack = createNativeStackNavigator<RootStackParamList>();

export default function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    checkAuthStatus();
  }, []);

  const checkAuthStatus = async () => {
    try {
      const token = await SecureStore.getItemAsync('github_token');
      setIsAuthenticated(!!token);
    } catch (error) {
      console.error('Error checking auth status:', error);
    } finally {
      setIsLoading(false);
    }
  };

  if (isLoading) {
    return null; // Or a loading screen component
  }

  return (
    <SafeAreaProvider>
      <AuthProvider>
        <ApiProvider>
          <NavigationContainer>
            <Stack.Navigator
              initialRouteName={isAuthenticated ? "CodespaceAccess" : "Auth"}
              screenOptions={{
                headerStyle: {
                  backgroundColor: '#0a0a0a',
                },
                headerTintColor: '#fff',
                contentStyle: {
                  backgroundColor: '#0a0a0a',
                },
              }}
            >
              <Stack.Screen
                name="Auth"
                component={AuthScreen}
                options={{ headerShown: false }}
              />
              <Stack.Screen
                name="CodespaceAccess"
                component={CodespaceAccessScreen}
                options={{ title: 'Catnip' }}
              />
              <Stack.Screen
                name="WorkspaceList"
                component={WorkspaceListScreen}
                options={{ title: 'Workspaces' }}
              />
              <Stack.Screen
                name="WorkspaceDetail"
                component={WorkspaceDetailScreen}
                options={({ route }) => ({
                  title: route.params.worktreeName.split('/')[1] || 'Workspace'
                })}
              />
            </Stack.Navigator>
          </NavigationContainer>
          <StatusBar style="light" />
        </ApiProvider>
      </AuthProvider>
    </SafeAreaProvider>
  );
}