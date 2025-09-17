import { useEffect, useState } from 'react';
import * as WebBrowser from 'expo-web-browser';
import * as AuthSession from 'expo-auth-session';
import * as SecureStore from 'expo-secure-store';
import { makeRedirectUri } from 'expo-auth-session';

WebBrowser.maybeCompleteAuthSession();

const CATNIP_BASE_URL = 'https://catnip.run';

export interface AuthState {
  isAuthenticated: boolean;
  isLoading: boolean;
  githubToken: string | null;
  githubUser: string | null;
  codespaceToken: string | null;
}

export function useAuth() {
  const [authState, setAuthState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    githubToken: null,
    githubUser: null,
    codespaceToken: null,
  });

  // Use Expo's redirect URI
  const redirectUri = makeRedirectUri({
    scheme: 'catnip',
    path: 'auth'
  });

  // Create the auth request
  const request = new AuthSession.AuthRequest({
    clientId: process.env.EXPO_PUBLIC_GITHUB_CLIENT_ID || '',
    scopes: ['read:user', 'user:email', 'repo', 'codespace'],
    redirectUri,
    responseType: AuthSession.ResponseType.Token,
    authorizationEndpoint: 'https://github.com/login/oauth/authorize',
  });

  // Load stored tokens on mount
  useEffect(() => {
    loadStoredAuth();
  }, []);

  const loadStoredAuth = async () => {
    try {
      const [token, user, csToken] = await Promise.all([
        SecureStore.getItemAsync('github_token'),
        SecureStore.getItemAsync('github_user'),
        SecureStore.getItemAsync('codespace_token'),
      ]);

      setAuthState({
        isAuthenticated: !!token,
        isLoading: false,
        githubToken: token,
        githubUser: user,
        codespaceToken: csToken,
      });
    } catch (error) {
      console.error('Failed to load auth:', error);
      setAuthState(prev => ({ ...prev, isLoading: false }));
    }
  };

  const login = async () => {
    try {
      // Start the auth flow
      const result = await request.promptAsync();

      if (result.type === 'success' && result.params.access_token) {
        const token = result.params.access_token;

        // Get user info
        const userResponse = await fetch('https://api.github.com/user', {
          headers: { Authorization: `Bearer ${token}` },
        });
        const userData = await userResponse.json();

        // Store credentials
        await Promise.all([
          SecureStore.setItemAsync('github_token', token),
          SecureStore.setItemAsync('github_user', userData.login),
        ]);

        setAuthState({
          isAuthenticated: true,
          isLoading: false,
          githubToken: token,
          githubUser: userData.login,
          codespaceToken: null,
        });

        return true;
      }
      return false;
    } catch (error) {
      console.error('Login failed:', error);
      return false;
    }
  };

  const logout = async () => {
    try {
      await Promise.all([
        SecureStore.deleteItemAsync('github_token'),
        SecureStore.deleteItemAsync('github_user'),
        SecureStore.deleteItemAsync('codespace_token'),
      ]);

      setAuthState({
        isAuthenticated: false,
        isLoading: false,
        githubToken: null,
        githubUser: null,
        codespaceToken: null,
      });
    } catch (error) {
      console.error('Logout failed:', error);
    }
  };

  const setCodespaceToken = async (token: string) => {
    await SecureStore.setItemAsync('codespace_token', token);
    setAuthState(prev => ({ ...prev, codespaceToken: token }));
  };

  return {
    ...authState,
    login,
    logout,
    setCodespaceToken,
  };
}