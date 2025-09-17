import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import * as SecureStore from 'expo-secure-store';
import * as WebBrowser from 'expo-web-browser';
import * as Linking from 'expo-linking';
import Constants from 'expo-constants';

interface AuthContextType {
  isAuthenticated: boolean;
  githubToken: string | null;
  githubUser: string | null;
  login: () => Promise<void>;
  logout: () => Promise<void>;
  setTokens: (token: string, user: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

interface AuthProviderProps {
  children: ReactNode;
}

export const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [githubToken, setGithubToken] = useState<string | null>(null);
  const [githubUser, setGithubUser] = useState<string | null>(null);

  useEffect(() => {
    loadStoredAuth();
  }, []);

  const loadStoredAuth = async () => {
    try {
      const token = await SecureStore.getItemAsync('github_token');
      const user = await SecureStore.getItemAsync('github_user');

      if (token && user) {
        setGithubToken(token);
        setGithubUser(user);
        setIsAuthenticated(true);
      }
    } catch (error) {
      console.error('Error loading stored auth:', error);
    }
  };

  const login = async () => {
    try {
      // Construct OAuth URL
      const redirectUri = Linking.createURL('/auth/callback');
      const authUrl = `https://catnip.run/v1/auth/github?redirect_uri=${encodeURIComponent(redirectUri)}`;

      // Open web browser for OAuth flow
      const result = await WebBrowser.openAuthSessionAsync(authUrl, redirectUri);

      if (result.type === 'success' && result.url) {
        // Parse the callback URL to extract token and user info
        const url = new URL(result.url);
        const token = url.searchParams.get('token');
        const user = url.searchParams.get('user');

        if (token && user) {
          await setTokens(token, user);
        }
      }
    } catch (error) {
      console.error('Login error:', error);
      throw error;
    }
  };

  const logout = async () => {
    try {
      await SecureStore.deleteItemAsync('github_token');
      await SecureStore.deleteItemAsync('github_user');
      await SecureStore.deleteItemAsync('codespace_token');
      setGithubToken(null);
      setGithubUser(null);
      setIsAuthenticated(false);
    } catch (error) {
      console.error('Logout error:', error);
    }
  };

  const setTokens = async (token: string, user: string) => {
    try {
      await SecureStore.setItemAsync('github_token', token);
      await SecureStore.setItemAsync('github_user', user);
      setGithubToken(token);
      setGithubUser(user);
      setIsAuthenticated(true);
    } catch (error) {
      console.error('Error storing tokens:', error);
      throw error;
    }
  };

  return (
    <AuthContext.Provider value={{
      isAuthenticated,
      githubToken,
      githubUser,
      login,
      logout,
      setTokens,
    }}>
      {children}
    </AuthContext.Provider>
  );
};