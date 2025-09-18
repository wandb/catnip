import * as SecureStore from 'expo-secure-store';

const BASE_URL = 'https://catnip.run';

export interface CodespaceInfo {
  name: string;
  lastUsed: number;
  repository?: string;
}

export interface WorkspaceInfo {
  id: string;
  name: string;
  branch: string;
  repository: string;
  claude_activity_state?: 'active' | 'running' | 'inactive';
  commit_count?: number;
  is_dirty?: boolean;
  last_accessed?: string;
  created_at?: string;
  todos?: Todo[];
  latest_claude_message?: string;
  pull_request_url?: string;
}

export interface Todo {
  content: string;
  status: 'pending' | 'in_progress' | 'completed';
  activeForm?: string;
}

class CatnipAPI {
  private async getHeaders(): Promise<HeadersInit> {
    const sessionToken = await SecureStore.getItemAsync('session_token');

    if (!sessionToken) {
      throw new Error('No session token available');
    }

    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${sessionToken}`,
    };
  }

  async connectCodespace(
    codespaceName?: string,
    org?: string,
    onEvent?: (event: any) => void
  ): Promise<void> {
    const headers = await this.getHeaders();
    const baseUrl = org ? `https://${org}.catnip.run` : BASE_URL;
    const url = codespaceName
      ? `${baseUrl}/v1/codespace?codespace=${encodeURIComponent(codespaceName)}`
      : `${baseUrl}/v1/codespace`;

    return new Promise((resolve, reject) => {
      // Since React Native doesn't have built-in EventSource, we'll use fetch with streaming
      // For production, you'd want to use a library like react-native-sse
      fetch(url, {
        headers: {
          ...headers,
          'Accept': 'text/event-stream',
        },
      })
        .then(async (response) => {
          if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
          }

          const reader = response.body?.getReader();
          if (!reader) {
            throw new Error('No response body');
          }

          const decoder = new TextDecoder();
          let buffer = '';

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
              if (line.startsWith('event: ')) {
                const eventType = line.substring(7);
                const nextLine = lines[lines.indexOf(line) + 1];

                if (nextLine?.startsWith('data: ')) {
                  try {
                    const data = JSON.parse(nextLine.substring(6));
                    onEvent?.({ type: eventType, ...data });

                    if (eventType === 'success') {
                      resolve();
                      return;
                    } else if (eventType === 'error') {
                      reject(new Error(data.message));
                      return;
                    }
                  } catch (e) {
                    console.error('Failed to parse SSE data:', e);
                  }
                }
              }
            }
          }
        })
        .catch((error) => {
          reject(error);
        });
    });
  }

  async getWorkspaces(): Promise<WorkspaceInfo[]> {
    const headers = await this.getHeaders();
    const response = await fetch(`${BASE_URL}/v1/workspaces`, { headers });

    if (!response.ok) {
      throw new Error('Failed to fetch workspaces');
    }

    return response.json();
  }

  async getWorkspace(id: string): Promise<WorkspaceInfo> {
    const headers = await this.getHeaders();
    const response = await fetch(`${BASE_URL}/v1/workspaces/${id}`, { headers });

    if (!response.ok) {
      throw new Error('Failed to fetch workspace');
    }

    return response.json();
  }

  async sendPrompt(workspaceId: string, prompt: string): Promise<void> {
    const headers = await this.getHeaders();
    const response = await fetch(`${BASE_URL}/v1/workspaces/${workspaceId}/prompt`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ prompt }),
    });

    if (!response.ok) {
      throw new Error('Failed to send prompt');
    }
  }

  async getAuthStatus(): Promise<{ authenticated: boolean; user?: string }> {
    try {
      const headers = await this.getHeaders();
      const response = await fetch(`${BASE_URL}/v1/auth/status`, { headers });

      if (!response.ok) {
        return { authenticated: false };
      }

      return response.json();
    } catch {
      return { authenticated: false };
    }
  }
}

export const api = new CatnipAPI();