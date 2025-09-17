import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  TouchableOpacity,
  StyleSheet,
  FlatList,
  RefreshControl,
  ActivityIndicator,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useNavigation } from '@react-navigation/native';
import { NativeStackNavigationProp } from '@react-navigation/native-stack';
import { RootStackParamList } from '../../App';
import { useApi } from '../contexts/ApiContext';

type NavigationProp = NativeStackNavigationProp<RootStackParamList, 'WorkspaceList'>;

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
}

function WorkspaceCard({ workspace, onPress }: { workspace: WorkspaceInfo; onPress: () => void }) {
  const getStatusColor = () => {
    switch (workspace.claude_activity_state) {
      case 'active':
        return '#22c55e';
      case 'running':
        return '#eab308';
      case 'inactive':
        return '#666';
      default:
        return '#666';
    }
  };

  const getWorkspaceTitle = () => {
    const parts = workspace.name.split('/');
    return parts[1] || workspace.name;
  };

  const cleanBranch = workspace.branch.startsWith('/')
    ? workspace.branch.slice(1)
    : workspace.branch;

  return (
    <TouchableOpacity style={styles.card} onPress={onPress}>
      <View style={styles.cardHeader}>
        <View style={styles.cardTitleRow}>
          <View style={[styles.statusIndicator, { backgroundColor: getStatusColor() }]} />
          <Text style={styles.cardTitle}>{getWorkspaceTitle()}</Text>
        </View>
        {workspace.commit_count && workspace.commit_count > 0 && (
          <Text style={styles.commitCount}>+{workspace.commit_count}</Text>
        )}
      </View>
      <Text style={styles.cardSubtitle}>
        {workspace.repository}/{getWorkspaceTitle()} · {cleanBranch}
      </Text>
      {workspace.is_dirty && (
        <View style={styles.modifiedBadge}>
          <Text style={styles.modifiedText}>Modified</Text>
        </View>
      )}
      {workspace.todos && workspace.todos.length > 0 && (
        <View style={styles.todosContainer}>
          <Text style={styles.todosLabel}>Tasks:</Text>
          <View style={styles.todosProgress}>
            {workspace.todos.map((todo, index) => (
              <View
                key={index}
                style={[
                  styles.todoIndicator,
                  todo.status === 'completed' && styles.todoCompleted,
                  todo.status === 'in_progress' && styles.todoInProgress,
                ]}
              />
            ))}
          </View>
        </View>
      )}
      <TouchableOpacity style={styles.continueButton}>
        <Text style={styles.continueText}>
          {workspace.claude_activity_state === 'active' ? 'Continue' : 'Open'}
        </Text>
      </TouchableOpacity>
    </TouchableOpacity>
  );
}

export default function WorkspaceListScreen() {
  const navigation = useNavigation<NavigationProp>();
  const { fetchWorkspaces } = useApi();

  const [workspaces, setWorkspaces] = useState<WorkspaceInfo[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);

  useEffect(() => {
    loadWorkspaces();
  }, []);

  const loadWorkspaces = async () => {
    try {
      const data = await fetchWorkspaces();
      setWorkspaces(data.sort((a, b) => {
        const aTime = new Date(a.last_accessed || a.created_at || 0).getTime();
        const bTime = new Date(b.last_accessed || b.created_at || 0).getTime();
        return bTime - aTime;
      }));
    } catch (error) {
      console.error('Failed to load workspaces:', error);
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  };

  const handleRefresh = async () => {
    setIsRefreshing(true);
    await loadWorkspaces();
  };

  const handleWorkspacePress = (workspace: WorkspaceInfo) => {
    navigation.navigate('WorkspaceDetail', {
      workspaceId: workspace.id,
      worktreeName: workspace.name,
      repository: workspace.repository,
      branch: workspace.branch,
    });
  };

  if (isLoading) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.loadingContainer}>
          <ActivityIndicator size="large" color="#7c3aed" />
          <Text style={styles.loadingText}>Loading workspaces...</Text>
        </View>
      </SafeAreaView>
    );
  }

  if (workspaces.length === 0) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.emptyContainer}>
          <Text style={styles.emptyTitle}>No workspaces</Text>
          <Text style={styles.emptySubtitle}>Create a workspace to get started</Text>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Workspaces</Text>
        <Text style={styles.headerSubtitle}>{workspaces.length} workspaces</Text>
      </View>
      <FlatList
        data={workspaces}
        keyExtractor={(item) => item.id}
        renderItem={({ item }) => (
          <WorkspaceCard
            workspace={item}
            onPress={() => handleWorkspacePress(item)}
          />
        )}
        contentContainerStyle={styles.listContent}
        refreshControl={
          <RefreshControl
            refreshing={isRefreshing}
            onRefresh={handleRefresh}
            tintColor="#7c3aed"
          />
        }
      />
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
    fontSize: 24,
    fontWeight: 'bold',
    color: '#fff',
    marginBottom: 4,
  },
  headerSubtitle: {
    fontSize: 14,
    color: '#666',
  },
  listContent: {
    padding: 16,
  },
  card: {
    backgroundColor: '#1a1a1a',
    borderRadius: 12,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: '#333',
  },
  cardHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 8,
  },
  cardTitleRow: {
    flexDirection: 'row',
    alignItems: 'center',
    flex: 1,
  },
  statusIndicator: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginRight: 8,
  },
  cardTitle: {
    fontSize: 18,
    fontWeight: '600',
    color: '#fff',
  },
  commitCount: {
    fontSize: 12,
    color: '#666',
    fontFamily: 'monospace',
  },
  cardSubtitle: {
    fontSize: 14,
    color: '#666',
    marginBottom: 12,
  },
  modifiedBadge: {
    backgroundColor: '#333',
    borderRadius: 4,
    paddingHorizontal: 8,
    paddingVertical: 4,
    alignSelf: 'flex-start',
    marginBottom: 12,
  },
  modifiedText: {
    fontSize: 11,
    color: '#999',
    textTransform: 'uppercase',
  },
  todosContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 12,
  },
  todosLabel: {
    fontSize: 12,
    color: '#666',
    marginRight: 8,
  },
  todosProgress: {
    flexDirection: 'row',
    gap: 4,
  },
  todoIndicator: {
    width: 20,
    height: 4,
    borderRadius: 2,
    backgroundColor: '#333',
  },
  todoCompleted: {
    backgroundColor: '#22c55e',
  },
  todoInProgress: {
    backgroundColor: '#eab308',
  },
  continueButton: {
    backgroundColor: '#7c3aed',
    paddingVertical: 12,
    borderRadius: 8,
    alignItems: 'center',
  },
  continueText: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600',
  },
  loadingContainer: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
  loadingText: {
    color: '#666',
    marginTop: 16,
    fontSize: 16,
  },
  emptyContainer: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
  emptyTitle: {
    fontSize: 20,
    fontWeight: '600',
    color: '#fff',
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 16,
    color: '#666',
  },
});