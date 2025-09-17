import React, { createContext, useContext, ReactNode } from 'react';
import { useAuth } from './AuthContext';
import * as SecureStore from 'expo-secure-store';

interface CodespaceInfo {
  name: string;
  lastUsed: number;
  repository?: string;
}

interface WorkspaceInfo {
  id: string;
  name: string;
  branch: string;
  repository: string;
  claude_activity_state?: string;
  commit_count?: number;
  is_dirty?: boolean;
  last_accessed?: string;
  created_at?: string;
  todos?: Array<{
    content: string;
    status: 'pending' | 'in_progress' | 'completed';
  }>;
  latest_claude_message?: string;
}

interface ApiContextType {
  baseUrl: string;
  fetchCodespaces: (org?: string) => Promise<CodespaceInfo[]>;
  accessCodespace: (codespaceName?: string, org?: string) => AsyncGenerator<any>;
  fetchWorkspaces: () => Promise<WorkspaceInfo[]>;
  sendPrompt: (workspaceId: string, prompt: string) => Promise<void>;
  getWorkspaceStatus: (workspaceId: string) => Promise<WorkspaceInfo>;
}

const ApiContext = createContext<ApiContextType | undefined>(undefined);

export const useApi = () => {
  const context = useContext(ApiContext);
  if (!context) {
    throw new Error('useApi must be used within an ApiProvider');
  }
  return context;
};

interface ApiProviderProps {
  children: ReactNode;
}

export const ApiProvider: React.FC<ApiProviderProps> = ({ children }) => {
  const { githubToken, githubUser } = useAuth();
  const baseUrl = 'https://catnip.run';

  const getHeaders = async () => {
    const codespaceToken = await SecureStore.getItemAsync('codespace_token');
    return {
      'Authorization': codespaceToken ? `Bearer ${codespaceToken}` : `Bearer ${githubToken}`,
      'Content-Type': 'application/json',
    };
  };

  const fetchCodespaces = async (org?: string): Promise<CodespaceInfo[]> => {
    const url = org
      ? `https://${org}.catnip.run/v1/codespaces`
      : `${baseUrl}/v1/codespaces`;

    const headers = await getHeaders();
    const response = await fetch(url, { headers });

    if (!response.ok) {
      throw new Error('Failed to fetch codespaces');
    }

    return response.json();
  };

  async function* accessCodespace(codespaceName?: string, org?: string) {
    const baseUrl = org ? `https://${org}.catnip.run` : 'https://catnip.run';
    const url = codespaceName
      ? `${baseUrl}/v1/codespace?codespace=${encodeURIComponent(codespaceName)}`
      : `${baseUrl}/v1/codespace`;

    const headers = await getHeaders();
    const response = await fetch(url, {
      headers: {
        ...headers,
        'Accept': 'text/event-stream',
      },
    });

    if (!response.body) {
      throw new Error('No response body');
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          try {
            const data = JSON.parse(line.slice(6));
            yield data;
          } catch (e) {
            console.error('Failed to parse SSE data:', e);
          }
        }
      }
    }
  }

  const fetchWorkspaces = async (): Promise<WorkspaceInfo[]> => {
    const codespaceToken = await SecureStore.getItemAsync('codespace_token');
    if (!codespaceToken) {
      throw new Error('No codespace token available');
    }

    const response = await fetch(`${baseUrl}/v1/workspaces`, {
      headers: {
        'Authorization': `Bearer ${codespaceToken}`,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error('Failed to fetch workspaces');
    }

    return response.json();
  };

  const sendPrompt = async (workspaceId: string, prompt: string): Promise<void> => {
    const codespaceToken = await SecureStore.getItemAsync('codespace_token');
    if (!codespaceToken) {
      throw new Error('No codespace token available');
    }

    const response = await fetch(`${baseUrl}/v1/workspaces/${workspaceId}/prompt`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${codespaceToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ prompt }),
    });

    if (!response.ok) {
      throw new Error('Failed to send prompt');
    }
  };

  const getWorkspaceStatus = async (workspaceId: string): Promise<WorkspaceInfo> => {
    const codespaceToken = await SecureStore.getItemAsync('codespace_token');
    if (!codespaceToken) {
      throw new Error('No codespace token available');
    }

    const response = await fetch(`${baseUrl}/v1/workspaces/${workspaceId}`, {
      headers: {
        'Authorization': `Bearer ${codespaceToken}`,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      throw new Error('Failed to get workspace status');
    }

    return response.json();
  };

  return (
    <ApiContext.Provider value={{
      baseUrl,
      fetchCodespaces,
      accessCodespace,
      fetchWorkspaces,
      sendPrompt,
      getWorkspaceStatus,
    }}>
      {children}
    </ApiContext.Provider>
  );
};