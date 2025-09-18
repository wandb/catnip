import { useState, useEffect } from "react";
import {
  View,
  Text,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
  Alert,
  TextInput,
  Modal,
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
  console.log("🚀 NEW WorkspaceCard CODE LOADED");
  // Debug log to see what might be causing the Text rendering issue
  console.log("🐱 WorkspaceCard rendering:", {
    id: workspace.id,
    name: workspace.name,
    branch: workspace.branch,
    repo_id: workspace.repo_id,
    commit_count: workspace.commit_count,
    is_dirty: workspace.is_dirty,
  });
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
    // For worktrees, name is already the friendly name like "feature-api-docs"
    const name = workspace.name;
    if (typeof name === "string" && name.trim()) {
      return name;
    }
    return "Unnamed workspace";
  };

  const getCleanBranch = () => {
    const branch = workspace.branch;
    if (typeof branch === "string" && branch.trim()) {
      // Handle refs/catnip/name format
      if (branch.startsWith("refs/catnip/")) {
        return branch.replace("refs/catnip/", "");
      }
      // Handle leading slash
      if (branch.startsWith("/")) {
        return branch.slice(1);
      }
      return branch;
    }
    return "main";
  };
  const cleanBranch = getCleanBranch();

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
        {workspace.commit_count > 0 && (
          <Text style={styles.commitCount}>+{workspace.commit_count}</Text>
        )}
      </View>

      <Text style={styles.cardSubtitle}>
        {typeof workspace.repo_id === "string"
          ? workspace.repo_id
          : "Unknown repo"}{" "}
        · {cleanBranch}
      </Text>

      {!!workspace.is_dirty && (
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
                {
                  width: `${totalTodos > 0 ? (completedTodos / totalTodos) * 100 : 0}%`,
                },
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
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [repository, setRepository] = useState("");
  const [branch, setBranch] = useState("main");

  useEffect(() => {
    loadWorkspaces();
  }, []);

  const loadWorkspaces = async () => {
    try {
      setError(null);
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
      console.error("🎯 Failed to load workspaces:", error);
      setError(
        error instanceof Error ? error.message : "Failed to load workspaces",
      );
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
    // Pass workspace data directly through navigation params
    console.log("🐱 WorkspacePress:", {
      id: workspace.id,
      name: workspace.name,
      path: workspace.path,
    });
    const encodedId = encodeURIComponent(workspace.id);
    console.log("🐱 Navigating to:", `/workspace/${encodedId}`);
    router.push({
      pathname: `/workspace/${encodedId}`,
      params: { workspaceData: JSON.stringify(workspace) },
    });
  };

  const handleCreateWorkspace = async () => {
    if (!repository.trim()) {
      Alert.alert("Error", "Please enter a repository in format 'owner/repo'");
      return;
    }

    if (!repository.includes("/")) {
      Alert.alert("Error", "Repository must be in format 'owner/repo'");
      return;
    }

    setIsCreating(true);
    try {
      console.log("🎯 Creating workspace:", {
        repository,
        branch,
      });
      const newWorkspace = await api.createWorkspace(
        repository.trim(),
        branch.trim() || "main",
      );
      console.log("🎯 Created workspace:", newWorkspace);

      // Add the new workspace to the list
      setWorkspaces((prev) => [newWorkspace, ...prev]);

      // Reset modal state
      setShowCreateModal(false);
      setRepository("");
      setBranch("main");

      Alert.alert("Success", "Workspace created successfully!");
    } catch (error) {
      console.error("🎯 Failed to create workspace:", error);
      Alert.alert(
        "Error",
        `Failed to create workspace: ${error instanceof Error ? error.message : "Unknown error"}`,
      );
    } finally {
      setIsCreating(false);
    }
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

  if (error) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.emptyContainer}>
          <Text style={styles.emptyTitle}>Error loading workspaces</Text>
          <Text style={styles.emptySubtitle}>{error}</Text>
          <Pressable
            style={styles.retryButton}
            onPress={() => loadWorkspaces()}
          >
            <Text style={styles.retryButtonText}>Retry</Text>
          </Pressable>
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
          <Pressable
            style={styles.createButton}
            onPress={() => setShowCreateModal(true)}
          >
            <Text style={styles.createButtonText}>Create Workspace</Text>
          </Pressable>
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

      {/* Create Workspace Modal */}
      <Modal
        visible={showCreateModal}
        transparent={true}
        animationType="fade"
        onRequestClose={() => setShowCreateModal(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <Text style={styles.modalTitle}>Create Workspace</Text>

            <Text style={styles.inputLabel}>Repository *</Text>
            <TextInput
              style={styles.input}
              value={repository}
              onChangeText={setRepository}
              placeholder="owner/repo"
              placeholderTextColor="#666"
              autoCapitalize="none"
            />

            <Text style={styles.inputLabel}>Branch</Text>
            <TextInput
              style={styles.input}
              value={branch}
              onChangeText={setBranch}
              placeholder="main"
              placeholderTextColor="#666"
              autoCapitalize="none"
            />

            <View style={styles.modalButtons}>
              <Pressable
                style={[styles.modalButton, styles.cancelButton]}
                onPress={() => setShowCreateModal(false)}
              >
                <Text style={styles.cancelButtonText}>Cancel</Text>
              </Pressable>
              <Pressable
                style={[styles.modalButton, styles.createModalButton]}
                onPress={handleCreateWorkspace}
                disabled={isCreating}
              >
                {isCreating ? (
                  <ActivityIndicator size="small" color="#fff" />
                ) : (
                  <Text style={styles.createModalButtonText}>Create</Text>
                )}
              </Pressable>
            </View>
          </View>
        </View>
      </Modal>
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
    marginBottom: 24,
  },
  retryButton: {
    backgroundColor: "#dc2626",
    paddingHorizontal: 24,
    paddingVertical: 12,
    borderRadius: 12,
    alignItems: "center",
  },
  retryButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  createButton: {
    backgroundColor: "#7c3aed",
    paddingHorizontal: 24,
    paddingVertical: 12,
    borderRadius: 12,
    alignItems: "center",
  },
  createButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.8)",
    justifyContent: "center",
    alignItems: "center",
    padding: 20,
  },
  modalContent: {
    backgroundColor: "#1a1a1a",
    borderRadius: 16,
    padding: 24,
    width: "100%",
    maxWidth: 400,
    borderWidth: 1,
    borderColor: "#333",
  },
  modalTitle: {
    fontSize: 20,
    fontWeight: "600",
    color: "#fff",
    marginBottom: 24,
    textAlign: "center",
  },
  inputLabel: {
    fontSize: 14,
    fontWeight: "500",
    color: "#fff",
    marginBottom: 8,
    marginTop: 16,
  },
  input: {
    backgroundColor: "#333",
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 16,
    color: "#fff",
    borderWidth: 1,
    borderColor: "#555",
  },
  modalButtons: {
    flexDirection: "row",
    marginTop: 24,
    gap: 12,
  },
  modalButton: {
    flex: 1,
    paddingVertical: 12,
    borderRadius: 8,
    alignItems: "center",
  },
  cancelButton: {
    backgroundColor: "#333",
    borderWidth: 1,
    borderColor: "#555",
  },
  cancelButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "500",
  },
  createModalButton: {
    backgroundColor: "#7c3aed",
  },
  createModalButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
});
