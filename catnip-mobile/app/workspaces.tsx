import { useState, useEffect, useRef } from "react";
import {
  View,
  Text,
  FlatList,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
  Alert,
  Pressable,
  Animated,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useRouter, useNavigation } from "expo-router";
import { api, WorkspaceInfo } from "../lib/api";
import { IOSButton, GlassIconButton } from "../components/ui";
import { NewWorkspaceDrawer } from "../components/NewWorkspaceDrawer";
import { theme } from "../theme";
import { useFocusEffect } from "@react-navigation/native";
import { useHeaderHeight } from "@react-navigation/elements";
import React from "react";

function StatusIndicator({ status }: { status: string | undefined }) {
  const pulseAnim = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    if (status === "active") {
      const pulse = Animated.loop(
        Animated.sequence([
          Animated.timing(pulseAnim, {
            toValue: 0.4,
            duration: 1000,
            useNativeDriver: true,
          }),
          Animated.timing(pulseAnim, {
            toValue: 1,
            duration: 1000,
            useNativeDriver: true,
          }),
        ]),
      );
      pulse.start();
      return () => pulse.stop();
    }
  }, [status, pulseAnim]);

  const getIndicatorStyle = () => {
    switch (status) {
      case "active":
        return {
          backgroundColor: "#22c55e",
          borderWidth: 0,
        };
      case "running":
        return {
          backgroundColor: "#6b7280",
          borderWidth: 0,
        };
      default:
        return {
          backgroundColor: "transparent",
          borderWidth: 1,
          borderColor: "#d1d5db",
        };
    }
  };

  return (
    <Animated.View
      style={[
        styles.statusIndicator,
        getIndicatorStyle(),
        status === "active" && { opacity: pulseAnim },
      ]}
    />
  );
}

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

  const getStatusText = () => {
    switch (workspace.claude_activity_state) {
      case "active":
        return "Active now";
      case "running":
        return "Running";
      default:
        return "Inactive";
    }
  };

  const getTimeDisplay = () => {
    const lastAccessed = workspace.last_accessed || workspace.created_at;
    if (!lastAccessed) return "";

    const date = new Date(lastAccessed);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffDays === 0) {
      return date.toLocaleTimeString([], {
        hour: "numeric",
        minute: "2-digit",
      });
    } else if (diffDays === 1) {
      return "Yesterday";
    } else if (diffDays < 7) {
      return date.toLocaleDateString([], { weekday: "short" });
    } else {
      return date.toLocaleDateString([], { month: "short", day: "numeric" });
    }
  };

  return (
    <Pressable style={styles.card} onPress={onPress}>
      <View style={styles.cardContent}>
        <View style={styles.cardHeader}>
          <View style={styles.mainContent}>
            <View style={styles.titleRow}>
              <Text style={styles.cardTitle}>{getTitle()}</Text>
              <Text style={styles.timeText}>{getTimeDisplay()}</Text>
            </View>
            <View style={styles.subtitleRow}>
              <View style={styles.repoInfo}>
                <Text style={styles.repoText}>
                  {typeof workspace.repo_id === "string"
                    ? workspace.repo_id
                    : "Unknown repo"}
                </Text>
                <Text style={styles.branchText}>· {cleanBranch}</Text>
              </View>
              <StatusIndicator status={workspace.claude_activity_state} />
            </View>
            {(!!workspace.is_dirty || (workspace.commit_count ?? 0) > 0) && (
              <View style={styles.badgeRow}>
                {!!workspace.is_dirty && (
                  <View style={styles.badge}>
                    <Text style={styles.badgeText}>Modified</Text>
                  </View>
                )}
                {(workspace.commit_count ?? 0) > 0 && (
                  <View style={styles.commitBadge}>
                    <Text style={styles.commitText}>
                      +{workspace.commit_count ?? 0}
                    </Text>
                  </View>
                )}
              </View>
            )}
          </View>
        </View>
      </View>
    </Pressable>
  );
}

export default function WorkspacesScreen() {
  const router = useRouter();
  const navigation = useNavigation();
  const headerHeight = useHeaderHeight();
  const [workspaces, setWorkspaces] = useState<WorkspaceInfo[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreateDrawer, setShowCreateDrawer] = useState(false);

  useEffect(() => {
    loadWorkspaces();
  }, []);

  useFocusEffect(
    React.useCallback(() => {
      navigation.setOptions({
        headerLeft: () => (
          <GlassIconButton
            icon="chevron-back"
            onPress={() => router.back()}
            color={theme.colors.brand.primary}
          />
        ),
        headerRight: () => (
          <GlassIconButton
            icon="add"
            onPress={() => setShowCreateDrawer(true)}
            color={theme.colors.brand.primary}
          />
        ),
      });
    }, [navigation]),
  );

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
      pathname: `/workspace/${encodedId}` as any,
      params: { workspaceData: JSON.stringify(workspace) },
    });
  };

  const handleCreateWorkspace = async (repository: string, branch: string) => {
    console.log("🎯 Creating workspace:", {
      repository,
      branch,
    });
    const newWorkspace = await api.createWorkspace(repository, branch);
    console.log("🎯 Created workspace:", newWorkspace);

    // Add the new workspace to the list
    setWorkspaces((prev) => [newWorkspace, ...prev]);
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
          <IOSButton
            title="Retry"
            onPress={() => loadWorkspaces()}
            variant="primary"
            style={styles.retryButton}
          />
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
          <IOSButton
            title="Create Workspace"
            onPress={() => setShowCreateDrawer(true)}
            variant="primary"
            style={styles.createButton}
          />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container} edges={["bottom", "left", "right"]}>
      <FlatList
        data={workspaces}
        keyExtractor={(item) => item.id}
        renderItem={({ item, index }) => (
          <>
            <WorkspaceCard
              workspace={item}
              onPress={() => handleWorkspacePress(item)}
            />
            {index < workspaces.length - 1 && <View style={styles.separator} />}
          </>
        )}
        contentContainerStyle={[
          styles.listContent,
          { paddingTop: headerHeight },
        ]}
        refreshControl={
          <RefreshControl
            refreshing={isRefreshing}
            onRefresh={handleRefresh}
            tintColor={theme.colors.brand.primary}
            colors={[theme.colors.brand.primary]}
          />
        }
      />

      <NewWorkspaceDrawer
        isOpen={showCreateDrawer}
        onClose={() => setShowCreateDrawer(false)}
        onCreateWorkspace={handleCreateWorkspace}
        existingWorkspaces={workspaces.map((w) => ({
          repo_id: w.repo_id,
          branch: w.branch,
        }))}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.colors.background.grouped,
  },
  listContent: {
    paddingBottom: theme.spacing.sm,
  },
  card: {
    backgroundColor: theme.colors.background.secondary,
    marginHorizontal: 0,
    marginBottom: 0,
    borderRadius: 0,
  },
  cardContent: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
  },
  separator: {
    height: 1,
    backgroundColor: theme.colors.separator.primary,
    marginLeft: theme.spacing.md,
    opacity: 0.5,
  },
  cardHeader: {
    flex: 1,
  },
  mainContent: {
    flex: 1,
  },
  titleRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
    marginBottom: 4,
  },
  cardTitle: {
    ...theme.typography.headline,
    color: theme.colors.text.primary,
    flex: 1,
    marginRight: theme.spacing.sm,
  },
  timeText: {
    ...theme.typography.callout,
    color: theme.colors.text.tertiary,
    fontSize: 15,
  },
  subtitleRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: theme.spacing.sm,
  },
  repoInfo: {
    flexDirection: "row",
    alignItems: "center",
    flex: 1,
  },
  repoText: {
    ...theme.typography.subheadline,
    color: theme.colors.text.secondary,
    marginRight: 4,
  },
  branchText: {
    ...theme.typography.subheadline,
    color: theme.colors.text.tertiary,
  },
  statusIndicator: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  badgeRow: {
    flexDirection: "row",
    gap: theme.spacing.xs,
  },
  badge: {
    backgroundColor: theme.colors.fill.secondary,
    borderRadius: theme.spacing.radius.xs,
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  badgeText: {
    ...theme.typography.caption2Emphasized,
    color: theme.colors.text.secondary,
    fontSize: 11,
    textTransform: "uppercase",
  },
  commitBadge: {
    backgroundColor: theme.colors.brand.primary + "20",
    borderRadius: theme.spacing.radius.xs,
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  commitText: {
    ...theme.typography.caption2Emphasized,
    color: theme.colors.brand.primary,
    fontSize: 11,
  },
  loadingContainer: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  loadingText: {
    ...theme.typography.body,
    color: theme.colors.text.secondary,
    marginTop: theme.spacing.md,
  },
  emptyContainer: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  emptyTitle: {
    ...theme.typography.title2,
    color: theme.colors.text.primary,
    marginBottom: theme.spacing.sm,
  },
  emptySubtitle: {
    ...theme.typography.body,
    color: theme.colors.text.secondary,
    marginBottom: theme.spacing.lg,
  },
  retryButton: {
    // Styles handled by IOSButton
  },
  createButton: {
    // Styles handled by IOSButton
  },
});
