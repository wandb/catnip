import { useEffect, useState } from 'react';
import * as WebBrowser from 'expo-web-browser';
import * as SecureStore from 'expo-secure-store';
import * as Linking from 'expo-linking';

WebBrowser.maybeCompleteAuthSession();

const CATNIP_BASE_URL = 'https://catnip.run';

export interface AuthState {
  isAuthenticated: boolean;
  isLoading: boolean;
  sessionToken: string | null;
  username: string | null;
}

export function useAuth() {
  const [authState, setAuthState] = useState<AuthState>({
    isAuthenticated: false,
    isLoading: true,
    sessionToken: null,
    username: null,
  });

  // Load stored session on mount
  useEffect(() => {
    loadStoredSession();
  }, []);

  // Set up deep linking listener for OAuth callback
  useEffect(() => {
    const handleCallback = async (url: string) => {
      const { queryParams } = Linking.parse(url);

      if (queryParams?.token && queryParams?.username) {
        // Store session token and username
        await SecureStore.setItemAsync('session_token', queryParams.token as string);
        await SecureStore.setItemAsync('username', queryParams.username as string);

        setAuthState({
          isAuthenticated: true,
          isLoading: false,
          sessionToken: queryParams.token as string,
          username: queryParams.username as string,
        });
      }
    };

    // Listen for incoming links
    const subscription = Linking.addEventListener('url', (event) => {
      handleCallback(event.url);
    });

    // Check if app was opened from a deep link
    Linking.getInitialURL().then((url) => {
      if (url) handleCallback(url);
    });

    return () => subscription.remove();
  }, []);

  const loadStoredSession = async () => {
    try {
      const [token, username] = await Promise.all([
        SecureStore.getItemAsync('session_token'),
        SecureStore.getItemAsync('username'),
      ]);

      // Validate session is still valid by checking with server
      if (token) {
        const response = await fetch(`${CATNIP_BASE_URL}/v1/auth/status`, {
          headers: {
            'Authorization': `Bearer ${token}`
          },
        });

        if (response.ok) {
          const data = await response.json();
          if (data.authenticated) {
            setAuthState({
              isAuthenticated: true,
              isLoading: false,
              sessionToken: token,
              username: username || data.username,
            });
            return;
          }
        }

        // Session is invalid, clean up
        await clearSession();
      }

      setAuthState(prev => ({ ...prev, isLoading: false }));
    } catch (error) {
      console.error('Failed to load session:', error);
      setAuthState(prev => ({ ...prev, isLoading: false }));
    }
  };

  const login = async () => {
    try {
      // Generate state for CSRF protection
      const state = Math.random().toString(36).substring(7);
      await SecureStore.setItemAsync('oauth_state', state);

      // Create redirect URI
      const redirectUri = Linking.createURL('auth');

      // Build OAuth URL - using our mobile OAuth relay endpoint
      const authUrl = `${CATNIP_BASE_URL}/v1/auth/github/mobile?` +
        `redirect_uri=${encodeURIComponent(redirectUri)}&` +
        `state=${state}`;

      // Open OAuth flow in browser
      const result = await WebBrowser.openAuthSessionAsync(authUrl, redirectUri);

      if (result.type === 'success' && result.url) {
        // Parse the callback URL
        const { queryParams } = Linking.parse(result.url);

        // Verify state matches
        const storedState = await SecureStore.getItemAsync('oauth_state');
        if (queryParams?.state !== storedState) {
          console.error('State mismatch in OAuth callback');
          return false;
        }

        if (queryParams?.token && queryParams?.username) {
          // Store session token and username
          await SecureStore.setItemAsync('session_token', queryParams.token as string);
          await SecureStore.setItemAsync('username', queryParams.username as string);

          setAuthState({
            isAuthenticated: true,
            isLoading: false,
            sessionToken: queryParams.token as string,
            username: queryParams.username as string,
          });

          return true;
        }
      }

      return false;
    } catch (error) {
      console.error('Login failed:', error);
      return false;
    }
  };

  const logout = async () => {
    try {
      // Notify server to revoke session
      if (authState.sessionToken) {
        await fetch(`${CATNIP_BASE_URL}/v1/auth/mobile/logout`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${authState.sessionToken}`,
            'Content-Type': 'application/json',
          },
        });
      }

      await clearSession();
    } catch (error) {
      console.error('Logout failed:', error);
      // Clear local session anyway
      await clearSession();
    }
  };

  const clearSession = async () => {
    await Promise.all([
      SecureStore.deleteItemAsync('session_token'),
      SecureStore.deleteItemAsync('username'),
      SecureStore.deleteItemAsync('oauth_state'),
    ]);

    setAuthState({
      isAuthenticated: false,
      isLoading: false,
      sessionToken: null,
      username: null,
    });
  };

  return {
    ...authState,
    login,
    logout,
  };
}