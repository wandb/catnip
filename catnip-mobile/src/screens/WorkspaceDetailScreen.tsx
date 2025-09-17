import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  TextInput,
  TouchableOpacity,
  StyleSheet,
  ScrollView,
  KeyboardAvoidingView,
  Platform,
  ActivityIndicator,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useRoute, RouteProp } from '@react-navigation/native';
import { RootStackParamList } from '../../App';
import { useApi } from '../contexts/ApiContext';

type RouteType = RouteProp<RootStackParamList, 'WorkspaceDetail'>;

type Phase = 'input' | 'todos' | 'completed' | 'existing' | 'error';

interface Todo {
  content: string;
  status: 'pending' | 'in_progress' | 'completed';
}

function TodoDisplay({ todos }: { todos: Todo[] }) {
  return (
    <View style={styles.todosContainer}>
      {todos.map((todo, index) => (
        <View key={index} style={styles.todoItem}>
          <View
            style={[
              styles.todoStatus,
              todo.status === 'completed' && styles.todoCompleted,
              todo.status === 'in_progress' && styles.todoInProgress,
            ]}
          />
          <Text style={styles.todoText}>{todo.content}</Text>
        </View>
      ))}
    </View>
  );
}

export default function WorkspaceDetailScreen() {
  const route = useRoute<RouteType>();
  const { workspaceId, worktreeName, repository, branch } = route.params;
  const { getWorkspaceStatus, sendPrompt } = useApi();

  const [phase, setPhase] = useState<Phase>('input');
  const [prompt, setPrompt] = useState('');
  const [todos, setTodos] = useState<Todo[]>([]);
  const [latestMessage, setLatestMessage] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [showNewPrompt, setShowNewPrompt] = useState(false);

  useEffect(() => {
    loadWorkspaceStatus();
  }, []);

  useEffect(() => {
    let interval: NodeJS.Timeout | null = null;

    if (phase === 'todos') {
      // Poll for status updates while Claude is working
      interval = setInterval(async () => {
        try {
          const status = await getWorkspaceStatus(workspaceId);
          if (status.todos) {
            setTodos(status.todos);
          }
          if (status.latest_claude_message) {
            setLatestMessage(status.latest_claude_message);
          }

          // Check if all todos are completed
          if (status.todos?.every(todo => todo.status === 'completed')) {
            setPhase('completed');
          }

          // Check if Claude is no longer active
          if (status.claude_activity_state === 'inactive') {
            setPhase('completed');
          }
        } catch (err) {
          console.error('Failed to get workspace status:', err);
        }
      }, 2000); // Poll every 2 seconds
    }

    return () => {
      if (interval) {
        clearInterval(interval);
      }
    };
  }, [phase, workspaceId]);

  const loadWorkspaceStatus = async () => {
    try {
      setIsLoading(true);
      const status = await getWorkspaceStatus(workspaceId);

      if (status.todos && status.todos.length > 0) {
        setTodos(status.todos);
        setPhase(status.claude_activity_state === 'active' ? 'todos' : 'existing');
      }

      if (status.latest_claude_message) {
        setLatestMessage(status.latest_claude_message);
      }
    } catch (err) {
      console.error('Failed to load workspace status:', err);
      setError('Failed to load workspace status');
    } finally {
      setIsLoading(false);
    }
  };

  const handleSendPrompt = async () => {
    if (!prompt.trim()) return;

    try {
      setIsLoading(true);
      setError('');
      await sendPrompt(workspaceId, prompt);
      setPrompt('');
      setShowNewPrompt(false);
      setPhase('todos');
    } catch (err) {
      console.error('Failed to send prompt:', err);
      setError('Failed to send prompt. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  const getWorkspaceTitle = () => {
    const parts = worktreeName.split('/');
    return parts[1] || worktreeName;
  };

  const cleanBranch = branch.startsWith('/') ? branch.slice(1) : branch;

  if (isLoading && phase === 'input') {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.loadingContainer}>
          <ActivityIndicator size="large" color="#7c3aed" />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <KeyboardAvoidingView
        style={styles.container}
        behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
      >
        <View style={styles.header}>
          <Text style={styles.headerTitle}>{getWorkspaceTitle()}</Text>
          <Text style={styles.headerSubtitle}>
            {repository} · {cleanBranch}
          </Text>
        </View>

        <ScrollView style={styles.content} contentContainerStyle={styles.contentContainer}>
          {phase === 'input' && (
            <View style={styles.inputPhase}>
              <Text style={styles.phaseTitle}>Start Working</Text>
              <Text style={styles.phaseSubtitle}>Describe what you'd like to work on</Text>
              <TextInput
                style={styles.textArea}
                placeholder="Describe your task..."
                placeholderTextColor="#666"
                value={prompt}
                onChangeText={setPrompt}
                multiline
                numberOfLines={6}
                textAlignVertical="top"
              />
            </View>
          )}

          {phase === 'todos' && (
            <View style={styles.todosPhase}>
              <View style={styles.statusContainer}>
                <ActivityIndicator size="small" color="#7c3aed" />
                <Text style={styles.statusText}>Claude is working on your request...</Text>
              </View>

              {latestMessage && (
                <View style={styles.sessionContext}>
                  <Text style={styles.contextLabel}>Session Context:</Text>
                  <Text style={styles.contextText}>{latestMessage}</Text>
                </View>
              )}

              {todos.length > 0 && (
                <View>
                  <Text style={styles.progressLabel}>Progress:</Text>
                  <TodoDisplay todos={todos} />
                </View>
              )}
            </View>
          )}

          {(phase === 'completed' || phase === 'existing') && (
            <View style={styles.completedPhase}>
              {latestMessage && (
                <View style={styles.messageContainer}>
                  <Text style={styles.messageText}>{latestMessage}</Text>
                </View>
              )}

              {todos.length > 0 && (
                <View>
                  <Text style={styles.progressLabel}>Tasks:</Text>
                  <TodoDisplay todos={todos} />
                </View>
              )}
            </View>
          )}

          {error && (
            <View style={styles.errorBox}>
              <Text style={styles.errorText}>{error}</Text>
            </View>
          )}
        </ScrollView>

        {phase === 'input' && (
          <View style={styles.footer}>
            <TouchableOpacity
              style={[styles.primaryButton, (!prompt.trim() || isLoading) && styles.buttonDisabled]}
              onPress={handleSendPrompt}
              disabled={!prompt.trim() || isLoading}
            >
              {isLoading ? (
                <ActivityIndicator color="#fff" />
              ) : (
                <Text style={styles.buttonText}>Start Working</Text>
              )}
            </TouchableOpacity>
          </View>
        )}

        {(phase === 'completed' || phase === 'existing') && (
          <View style={styles.footer}>
            {showNewPrompt ? (
              <>
                <TextInput
                  style={styles.promptInput}
                  placeholder="Describe what you'd like to change..."
                  placeholderTextColor="#666"
                  value={prompt}
                  onChangeText={setPrompt}
                  multiline
                  numberOfLines={3}
                  textAlignVertical="top"
                />
                <View style={styles.buttonRow}>
                  <TouchableOpacity
                    style={[styles.primaryButton, styles.flexButton]}
                    onPress={handleSendPrompt}
                    disabled={!prompt.trim() || isLoading}
                  >
                    <Text style={styles.buttonText}>Send</Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={[styles.secondaryButton, styles.flexButton]}
                    onPress={() => {
                      setShowNewPrompt(false);
                      setPrompt('');
                    }}
                  >
                    <Text style={styles.secondaryButtonText}>Cancel</Text>
                  </TouchableOpacity>
                </View>
              </>
            ) : (
              <TouchableOpacity
                style={styles.primaryButton}
                onPress={() => setShowNewPrompt(true)}
              >
                <Text style={styles.buttonText}>Ask for changes</Text>
              </TouchableOpacity>
            )}
          </View>
        )}
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0a0a0a',
  },
  header: {
    paddingHorizontal: 20,
    paddingVertical: 16,
    borderBottomWidth: 1,
    borderBottomColor: '#1a1a1a',
  },
  headerTitle: {
    fontSize: 20,
    fontWeight: '600',
    color: '#fff',
    marginBottom: 4,
  },
  headerSubtitle: {
    fontSize: 14,
    color: '#666',
  },
  content: {
    flex: 1,
  },
  contentContainer: {
    padding: 20,
  },
  inputPhase: {
    alignItems: 'center',
    marginTop: 40,
  },
  phaseTitle: {
    fontSize: 24,
    fontWeight: '600',
    color: '#fff',
    marginBottom: 8,
  },
  phaseSubtitle: {
    fontSize: 14,
    color: '#666',
    marginBottom: 24,
  },
  textArea: {
    width: '100%',
    backgroundColor: '#1a1a1a',
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 8,
    padding: 16,
    color: '#fff',
    fontSize: 14,
    minHeight: 120,
  },
  promptInput: {
    backgroundColor: '#1a1a1a',
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 8,
    padding: 12,
    color: '#fff',
    fontSize: 14,
    marginBottom: 12,
    minHeight: 80,
  },
  footer: {
    padding: 20,
    borderTopWidth: 1,
    borderTopColor: '#1a1a1a',
  },
  primaryButton: {
    backgroundColor: '#7c3aed',
    paddingVertical: 14,
    borderRadius: 8,
    alignItems: 'center',
  },
  secondaryButton: {
    backgroundColor: '#333',
    paddingVertical: 14,
    borderRadius: 8,
    alignItems: 'center',
  },
  buttonDisabled: {
    opacity: 0.5,
  },
  buttonText: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600',
  },
  secondaryButtonText: {
    color: '#ccc',
    fontSize: 16,
    fontWeight: '600',
  },
  buttonRow: {
    flexDirection: 'row',
    gap: 12,
  },
  flexButton: {
    flex: 1,
  },
  todosPhase: {
    marginTop: 20,
  },
  statusContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    marginBottom: 24,
  },
  statusText: {
    color: '#999',
    fontSize: 14,
  },
  sessionContext: {
    backgroundColor: 'rgba(124, 58, 237, 0.1)',
    borderWidth: 1,
    borderColor: 'rgba(124, 58, 237, 0.2)',
    borderRadius: 8,
    padding: 16,
    marginBottom: 24,
  },
  contextLabel: {
    fontSize: 12,
    color: 'rgba(124, 58, 237, 0.8)',
    marginBottom: 8,
    fontWeight: '600',
  },
  contextText: {
    color: '#ccc',
    fontSize: 14,
    lineHeight: 20,
  },
  progressLabel: {
    fontSize: 14,
    color: '#999',
    marginBottom: 12,
    fontWeight: '600',
  },
  todosContainer: {
    gap: 8,
  },
  todoItem: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    paddingVertical: 8,
  },
  todoStatus: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#333',
  },
  todoCompleted: {
    backgroundColor: '#22c55e',
  },
  todoInProgress: {
    backgroundColor: '#eab308',
  },
  todoText: {
    color: '#ccc',
    fontSize: 14,
    flex: 1,
  },
  completedPhase: {
    marginTop: 20,
  },
  messageContainer: {
    backgroundColor: '#1a1a1a',
    borderRadius: 8,
    padding: 16,
    marginBottom: 24,
  },
  messageText: {
    color: '#ccc',
    fontSize: 14,
    lineHeight: 20,
  },
  errorBox: {
    backgroundColor: 'rgba(239, 68, 68, 0.1)',
    borderWidth: 1,
    borderColor: 'rgba(239, 68, 68, 0.3)',
    borderRadius: 8,
    padding: 12,
    marginTop: 16,
  },
  errorText: {
    color: '#fca5a5',
    fontSize: 14,
  },
  loadingContainer: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
});