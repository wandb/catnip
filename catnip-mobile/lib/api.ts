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
    const codespaceToken = await SecureStore.getItemAsync('codespace_token');
    const githubToken = await SecureStore.getItemAsync('github_token');

    return {
      'Content-Type': 'application/json',
      'Authorization': codespaceToken
        ? `Bearer ${codespaceToken}`
        : githubToken
          ? `Bearer ${githubToken}`
          : '',
    };
  }

  async connectCodespace(
    codespaceName?: string,
    org?: string,
    onEvent?: (event: any) => void
  ): Promise<void> {
    const baseUrl = org ? `https://${org}.catnip.run` : BASE_URL;
    const url = codespaceName
      ? `${baseUrl}/v1/codespace?codespace=${encodeURIComponent(codespaceName)}`
      : `${baseUrl}/v1/codespace`;

    const headers = await this.getHeaders();

    return new Promise((resolve, reject) => {
      // Use EventSource for Server-Sent Events
      const eventSource = new EventSource(url);

      eventSource.addEventListener('status', (event: MessageEvent) => {
        const data = JSON.parse(event.data);
        onEvent?.({ type: 'status', ...data });
      });

      eventSource.addEventListener('success', async (event: MessageEvent) => {
        const data = JSON.parse(event.data);

        // Store codespace token if provided
        if (data.token) {
          await SecureStore.setItemAsync('codespace_token', data.token);
        }

        onEvent?.({ type: 'success', ...data });
        eventSource.close();
        resolve();
      });

      eventSource.addEventListener('error', (event: MessageEvent) => {
        const data = JSON.parse(event.data);
        onEvent?.({ type: 'error', ...data });
        eventSource.close();
        reject(new Error(data.message));
      });

      eventSource.addEventListener('setup', (event: MessageEvent) => {
        const data = JSON.parse(event.data);
        onEvent?.({ type: 'setup', ...data });
        eventSource.close();
        resolve();
      });

      eventSource.addEventListener('multiple', (event: MessageEvent) => {
        const data = JSON.parse(event.data);
        onEvent?.({ type: 'multiple', ...data });
        eventSource.close();
        resolve();
      });

      eventSource.onerror = () => {
        eventSource.close();
        reject(new Error('Connection failed'));
      };
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
    const headers = await this.getHeaders();
    const response = await fetch(`${BASE_URL}/v1/auth/status`, { headers });

    if (!response.ok) {
      return { authenticated: false };
    }

    return response.json();
  }
}

export const api = new CatnipAPI();