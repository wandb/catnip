import { useState, useEffect } from "react";
import {
  View,
  Text,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useRouter } from "expo-router";
import { api, WorkspaceInfo } from "../lib/api";

function WorkspaceCard({
  workspace,
  onPress,
}: {
  workspace: WorkspaceInfo;
  onPress: () => void;
}) {
  const getStatusColor = () => {
    switch (workspace.claude_activity_state) {
      case "active":
        return "#22c55e";
      case "running":
        return "#eab308";
      default:
        return "#666";
    }
  };

  const getTitle = () => {
    const parts = workspace.name.split("/");
    return parts[1] || workspace.name;
  };

  const cleanBranch = workspace.branch.startsWith("/")
    ? workspace.branch.slice(1)
    : workspace.branch;

  const completedTodos =
    workspace.todos?.filter((t) => t.status === "completed").length || 0;
  const totalTodos = workspace.todos?.length || 0;

  return (
    <Pressable style={styles.card} onPress={onPress}>
      <View style={styles.cardHeader}>
        <View style={styles.cardTitleRow}>
          <View
            style={[
              styles.statusIndicator,
              { backgroundColor: getStatusColor() },
            ]}
          />
          <Text style={styles.cardTitle}>{getTitle()}</Text>
        </View>
        {workspace.commit_count && workspace.commit_count > 0 && (
          <Text style={styles.commitCount}>+{workspace.commit_count}</Text>
        )}
      </View>

      <Text style={styles.cardSubtitle}>
        {workspace.repository}/{getTitle()} · {cleanBranch}
      </Text>

      {workspace.is_dirty && (
        <View style={styles.badge}>
          <Text style={styles.badgeText}>Modified</Text>
        </View>
      )}

      {totalTodos > 0 && (
        <View style={styles.progressContainer}>
          <Text style={styles.progressText}>
            Tasks: {completedTodos}/{totalTodos}
          </Text>
          <View style={styles.progressBar}>
            <View
              style={[
                styles.progressFill,
                { width: `${(completedTodos / totalTodos) * 100}%` },
              ]}
            />
          </View>
        </View>
      )}

      <View style={styles.cardButton}>
        <Text style={styles.cardButtonText}>
          {workspace.claude_activity_state === "active" ? "Continue" : "Open"}
        </Text>
      </View>
    </Pressable>
  );
}

export default function WorkspacesScreen() {
  const router = useRouter();
  const [workspaces, setWorkspaces] = useState<WorkspaceInfo[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);

  useEffect(() => {
    loadWorkspaces();
  }, []);

  const loadWorkspaces = async () => {
    try {
      const data = await api.getWorkspaces();
      setWorkspaces(
        data.sort((a, b) => {
          const aTime = new Date(
            a.last_accessed || a.created_at || 0,
          ).getTime();
          const bTime = new Date(
            b.last_accessed || b.created_at || 0,
          ).getTime();
          return bTime - aTime;
        }),
      );
    } catch (error) {
      console.error("Failed to load workspaces:", error);
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
    router.push(`/workspace/${workspace.id}`);
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
          <Text style={styles.emptySubtitle}>
            Create a workspace to get started
          </Text>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container} edges={["top"]}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>Workspaces</Text>
        <Text style={styles.headerSubtitle}>
          {workspaces.length} workspaces
        </Text>
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
            colors={["#7c3aed"]}
          />
        }
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#0a0a0a",
  },
  header: {
    paddingHorizontal: 20,
    paddingVertical: 16,
    borderBottomWidth: 1,
    borderBottomColor: "#1a1a1a",
  },
  headerTitle: {
    fontSize: 24,
    fontWeight: "bold",
    color: "#fff",
    marginBottom: 4,
  },
  headerSubtitle: {
    fontSize: 14,
    color: "#666",
  },
  listContent: {
    padding: 16,
  },
  card: {
    backgroundColor: "#1a1a1a",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#333",
  },
  cardHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 8,
  },
  cardTitleRow: {
    flexDirection: "row",
    alignItems: "center",
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
    fontWeight: "600",
    color: "#fff",
  },
  commitCount: {
    fontSize: 12,
    color: "#666",
  },
  cardSubtitle: {
    fontSize: 14,
    color: "#666",
    marginBottom: 12,
  },
  badge: {
    backgroundColor: "#333",
    borderRadius: 6,
    paddingHorizontal: 8,
    paddingVertical: 4,
    alignSelf: "flex-start",
    marginBottom: 12,
  },
  badgeText: {
    fontSize: 11,
    color: "#999",
    textTransform: "uppercase",
  },
  progressContainer: {
    marginBottom: 12,
  },
  progressText: {
    fontSize: 12,
    color: "#999",
    marginBottom: 6,
  },
  progressBar: {
    height: 4,
    backgroundColor: "#333",
    borderRadius: 2,
    overflow: "hidden",
  },
  progressFill: {
    height: "100%",
    backgroundColor: "#7c3aed",
  },
  cardButton: {
    backgroundColor: "#7c3aed",
    paddingVertical: 12,
    borderRadius: 12,
    alignItems: "center",
  },
  cardButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  loadingContainer: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  loadingText: {
    color: "#666",
    marginTop: 16,
    fontSize: 16,
  },
  emptyContainer: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  emptyTitle: {
    fontSize: 20,
    fontWeight: "600",
    color: "#fff",
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 16,
    color: "#666",
  },
});
