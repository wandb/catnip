import { useState, useEffect, useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  KeyboardAvoidingView,
  Platform,
  StyleSheet,
  ActivityIndicator,
  TouchableWithoutFeedback,
  Keyboard,
  InputAccessoryView,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useNavigation, useRouter } from "expo-router";
import { useHeaderHeight } from "@react-navigation/elements";
import { useFocusEffect } from "@react-navigation/native";
import { api, WorkspaceInfo, Todo } from "../../lib/api";
import { GlassInput, IOSButton, GlassIconButton } from "../../components/ui";
import { theme } from "../../theme";
import React from "react";

type Phase = "loading" | "input" | "working" | "completed" | "error";

function TodoList({ todos }: { todos: Todo[] }) {
  return (
    <View style={styles.todosContainer}>
      {todos.map((todo, index) => (
        <View key={index} style={styles.todoItem}>
          <View
            style={[
              styles.todoStatus,
              todo.status === "completed" && styles.todoCompleted,
              todo.status === "in_progress" && styles.todoInProgress,
            ]}
          />
          <Text style={styles.todoText}>{todo.content}</Text>
        </View>
      ))}
    </View>
  );
}

export default function WorkspaceDetailScreen() {
  const { id, workspaceData } = useLocalSearchParams<{
    id: string;
    workspaceData?: string;
  }>();
  const router = useRouter();
  const navigation = useNavigation();
  const headerHeight = useHeaderHeight();
  const inputAccessoryViewID = "workspace-input";

  console.log("🐱 WorkspaceDetailScreen loaded with ID:", id);

  const [workspace, setWorkspace] = useState<WorkspaceInfo | null>(null);
  const [phase, setPhase] = useState<Phase>("loading");
  const [prompt, setPrompt] = useState("");
  const [showPromptInput, setShowPromptInput] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");

  const loadWorkspace = useCallback(async () => {
    if (!id) return;

    try {
      let data: WorkspaceInfo;

      // Try to use passed workspace data first
      if (workspaceData) {
        try {
          data = JSON.parse(workspaceData);
          console.log("🐱 Using passed workspace data:", data);
        } catch (parseError) {
          console.error(
            "🐱 Failed to parse workspace data, falling back to API:",
            parseError,
          );
          const decodedId = decodeURIComponent(id);
          data = await api.getWorkspace(decodedId);
        }
      } else {
        console.log("🐱 No workspace data passed, using API");
        const decodedId = decodeURIComponent(id);
        data = await api.getWorkspace(decodedId);
      }

      setWorkspace(data);

      // Load Claude session data and latest message
      try {
        const sessions = await api.getClaudeSessions();
        const sessionData = sessions[data.path];

        if (sessionData && sessionData.turnCount > 0) {
          console.log("🐱 Found Claude session for workspace:", sessionData);

          // Get the latest message
          const messageResult = await api.getWorktreeLatestMessage(data.path);
          if (messageResult.isError) {
            setError(messageResult.content);
            setPhase("error");
          } else {
            // Update workspace with latest Claude data
            setWorkspace((prev) =>
              prev
                ? {
                    ...prev,
                    latest_session_title: messageResult.content,
                    claude_activity_state: sessionData.isActive
                      ? "active"
                      : "inactive",
                  }
                : prev,
            );

            // Determine phase based on Claude activity
            if (sessionData.isActive) {
              setPhase("working");
            } else {
              setPhase("completed");
            }
          }
        } else {
          // No Claude session found - determine phase based on workspace state
          if (data.claude_activity_state === "active") {
            setPhase("working");
          } else if (
            data.latest_session_title ||
            (data.todos && data.todos.length > 0)
          ) {
            setPhase("completed");
          } else {
            setPhase("input");
          }
        }
      } catch (claudeError) {
        console.warn("Failed to load Claude data:", claudeError);
        // Fall back to workspace-based phase determination
        if (data.claude_activity_state === "active") {
          setPhase("working");
        } else if (
          data.latest_session_title ||
          (data.todos && data.todos.length > 0)
        ) {
          setPhase("completed");
        } else {
          setPhase("input");
        }
      }
    } catch (err: any) {
      console.error("Failed to load workspace:", err);
      setError(err.message || "Failed to load workspace");
      setPhase("error");
    }
  }, [id, workspaceData]);

  useEffect(() => {
    loadWorkspace();
  }, [loadWorkspace]);

  // Poll for updates when workspace is active
  useEffect(() => {
    if (phase !== "working" || !workspace) return;

    const interval = setInterval(async () => {
      try {
        // Check Claude session status
        const sessions = await api.getClaudeSessions();
        const sessionData = sessions[workspace.path];

        if (sessionData) {
          // Get the latest message
          const messageResult = await api.getWorktreeLatestMessage(
            workspace.path,
          );

          if (messageResult.isError) {
            setError(messageResult.content);
            setPhase("error");
            return;
          }

          // Update workspace with latest data
          setWorkspace((prev) =>
            prev
              ? {
                  ...prev,
                  latest_session_title: messageResult.content,
                  claude_activity_state: sessionData.isActive
                    ? "active"
                    : "inactive",
                }
              : prev,
          );

          // Check if work is completed
          if (!sessionData.isActive) {
            setPhase("completed");
          }
        } else {
          // No session data - refresh workspace data
          const decodedId = decodeURIComponent(id);
          const refreshedData = await api.getWorkspace(decodedId);
          setWorkspace(refreshedData);

          if (refreshedData.claude_activity_state === "inactive") {
            setPhase("completed");
          }
        }
      } catch (err) {
        console.error("Failed to poll workspace updates:", err);
        // Don't change phase on polling errors - just log them
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [phase, id, workspace?.path]);

  // Set custom header buttons and title
  useFocusEffect(
    React.useCallback(() => {
      if (workspace) {
        const title = workspace.name.split("/")[1] || workspace.name;
        navigation.setOptions({
          title,
          headerLeft: () => (
            <GlassIconButton
              icon="chevron-back"
              onPress={() => router.back()}
              color={theme.colors.brand.primary}
            />
          ),
        });
      }
    }, [workspace, navigation, router]),
  );

  const handleSendPrompt = async () => {
    if (!prompt.trim() || !workspace) return;

    setIsSubmitting(true);
    setError("");

    try {
      await api.sendPrompt(workspace.path, prompt.trim());
      setPrompt("");
      setShowPromptInput(false);
      setPhase("working");
    } catch (err: any) {
      setError(err.message || "Failed to send prompt");
    } finally {
      setIsSubmitting(false);
    }
  };

  if (phase === "loading") {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.centerContainer}>
          <ActivityIndicator size="large" color="#7c3aed" />
          <Text style={styles.loadingText}>Loading workspace...</Text>
        </View>
      </SafeAreaView>
    );
  }

  if (phase === "error" || !workspace) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.centerContainer}>
          <Text style={styles.errorTitle}>Error</Text>
          <Text style={styles.errorText}>{error || "Workspace not found"}</Text>
          <IOSButton
            title="Retry"
            onPress={loadWorkspace}
            variant="primary"
            style={styles.retryButton}
          />
        </View>
      </SafeAreaView>
    );
  }

  const cleanBranch = workspace.branch.startsWith("/")
    ? workspace.branch.slice(1)
    : workspace.branch;

  return (
    <SafeAreaView style={styles.container} edges={["bottom", "left", "right"]}>
      <TouchableWithoutFeedback onPress={Keyboard.dismiss}>
        <KeyboardAvoidingView
          style={styles.container}
          behavior={Platform.OS === "ios" ? "padding" : "height"}
        >
          <ScrollView
            style={styles.content}
            contentContainerStyle={[
              styles.contentContainer,
              { paddingTop: headerHeight + theme.spacing.xl },
            ]}
            keyboardShouldPersistTaps="handled"
          >
            {phase === "input" && (
              <View style={styles.inputSection}>
                <Text style={styles.sectionTitle}>Start Working</Text>
                <Text style={styles.sectionSubtitle}>
                  Describe what you'd like to work on
                </Text>
                <GlassInput
                  placeholder="Describe your task..."
                  value={prompt}
                  onChangeText={setPrompt}
                  multiline
                  style={styles.promptInput}
                  inputAccessoryViewID={
                    Platform.OS === "ios" ? inputAccessoryViewID : undefined
                  }
                />
              </View>
            )}

            {phase === "working" && (
              <View style={styles.workingSection}>
                <View style={styles.statusContainer}>
                  <ActivityIndicator
                    size="small"
                    color={theme.colors.brand.primary}
                  />
                  <Text style={styles.statusText}>Claude is working...</Text>
                </View>

                {workspace.latest_session_title && (
                  <View style={styles.messageBox}>
                    <Text style={styles.messageLabel}>Session:</Text>
                    <Text style={styles.messageText}>
                      {workspace.latest_session_title}
                    </Text>
                  </View>
                )}

                {workspace.todos && workspace.todos.length > 0 && (
                  <View>
                    <Text style={styles.sectionLabel}>Progress:</Text>
                    <TodoList todos={workspace.todos} />
                  </View>
                )}
              </View>
            )}

            {phase === "completed" && (
              <View style={styles.completedSection}>
                {workspace.latest_session_title && (
                  <View style={styles.messageBox}>
                    <Text style={styles.messageText}>
                      {workspace.latest_session_title}
                    </Text>
                  </View>
                )}

                {workspace.todos && workspace.todos.length > 0 && (
                  <View>
                    <Text style={styles.sectionLabel}>Tasks:</Text>
                    <TodoList todos={workspace.todos} />
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

          <View style={styles.footer}>
            {phase === "input" && (
              <IOSButton
                title="Start Working"
                onPress={handleSendPrompt}
                disabled={!prompt.trim() || isSubmitting || !workspace}
                loading={isSubmitting}
                variant="primary"
                size="large"
              />
            )}

            {phase === "completed" && (
              <>
                {showPromptInput ? (
                  <View style={styles.promptInputContainer}>
                    <GlassInput
                      placeholder="Describe what you'd like to change..."
                      value={prompt}
                      onChangeText={setPrompt}
                      multiline
                      style={styles.bottomPromptInput}
                      autoFocus
                      inputAccessoryViewID={
                        Platform.OS === "ios" ? inputAccessoryViewID : undefined
                      }
                    />
                    <IOSButton
                      title="Cancel"
                      onPress={() => {
                        setShowPromptInput(false);
                        setPrompt("");
                        Keyboard.dismiss();
                      }}
                      variant="secondary"
                    />
                  </View>
                ) : (
                  <IOSButton
                    title="Ask for changes"
                    onPress={() => setShowPromptInput(true)}
                    variant="primary"
                  />
                )}
              </>
            )}
          </View>
        </KeyboardAvoidingView>
      </TouchableWithoutFeedback>

      {Platform.OS === "ios" && (
        <InputAccessoryView nativeID={inputAccessoryViewID}>
          <View style={styles.inputAccessory}>
            <TouchableWithoutFeedback
              onPress={() => {
                if (!prompt.trim() || isSubmitting || !workspace) return;
                handleSendPrompt();
              }}
            >
              <View
                style={[
                  styles.accessoryButton,
                  (!prompt.trim() || isSubmitting || !workspace) &&
                    styles.accessoryButtonDisabled,
                ]}
              >
                <Text style={styles.accessoryButtonText}>↑</Text>
              </View>
            </TouchableWithoutFeedback>
          </View>
        </InputAccessoryView>
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.colors.background.grouped,
  },
  content: {
    flex: 1,
  },
  contentContainer: {
    paddingBottom: theme.spacing.lg,
  },
  inputSection: {
    backgroundColor: theme.colors.background.secondary,
    marginHorizontal: theme.spacing.md,
    borderRadius: theme.spacing.radius.lg,
    padding: theme.spacing.md,
    paddingTop: theme.spacing.lg,
    paddingBottom: theme.spacing.lg,
  },
  sectionTitle: {
    ...theme.typography.title1,
    color: theme.colors.text.primary,
    marginBottom: theme.spacing.sm,
    textAlign: "center",
  },
  sectionSubtitle: {
    ...theme.typography.body,
    color: theme.colors.text.secondary,
    marginBottom: theme.spacing.lg,
    textAlign: "center",
  },
  promptInput: {
    width: "100%",
    minHeight: 120,
  },
  workingSection: {
    backgroundColor: theme.colors.background.secondary,
    marginHorizontal: theme.spacing.md,
    borderRadius: theme.spacing.radius.lg,
    padding: theme.spacing.component.cardPadding,
  },
  statusContainer: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
    marginBottom: theme.spacing.lg,
  },
  statusText: {
    ...theme.typography.callout,
    color: theme.colors.text.secondary,
  },
  messageBox: {
    marginBottom: theme.spacing.lg,
    backgroundColor: theme.colors.fill.secondary,
    borderRadius: theme.spacing.radius.md,
    padding: theme.spacing.md,
  },
  messageLabel: {
    ...theme.typography.caption1Emphasized,
    color: theme.colors.brand.primary,
    marginBottom: theme.spacing.sm,
  },
  messageText: {
    ...theme.typography.body,
    color: theme.colors.text.primary,
    lineHeight: 20,
  },
  completedSection: {
    backgroundColor: theme.colors.background.secondary,
    marginHorizontal: theme.spacing.md,
    borderRadius: theme.spacing.radius.lg,
    padding: theme.spacing.component.cardPadding,
  },
  sectionLabel: {
    ...theme.typography.calloutEmphasized,
    color: theme.colors.text.secondary,
    marginBottom: theme.spacing.md,
  },
  todosContainer: {
    gap: theme.spacing.sm,
  },
  todoItem: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
  },
  todoStatus: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: theme.colors.fill.tertiary,
    marginTop: 6,
  },
  todoCompleted: {
    backgroundColor: theme.colors.status.success,
  },
  todoInProgress: {
    backgroundColor: theme.colors.status.warning,
  },
  todoText: {
    ...theme.typography.body,
    color: theme.colors.text.primary,
    flex: 1,
    lineHeight: 20,
  },
  footer: {
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.lg,
    paddingBottom: theme.spacing.xl,
  },
  promptInputContainer: {
    gap: theme.spacing.md,
  },
  bottomPromptInput: {
    minHeight: 80,
  },
  errorBox: {
    backgroundColor: `${theme.colors.status.error}1A`, // 10% opacity
    borderWidth: theme.spacing.borderWidth.thin,
    borderColor: `${theme.colors.status.error}4D`, // 30% opacity
    borderRadius: theme.spacing.radius.md,
    padding: theme.spacing.md,
    marginHorizontal: theme.spacing.md,
    marginTop: theme.spacing.md,
  },
  errorText: {
    ...theme.typography.callout,
    color: theme.colors.status.error,
  },
  centerContainer: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: theme.spacing.lg,
  },
  loadingText: {
    ...theme.typography.body,
    color: theme.colors.text.secondary,
    marginTop: theme.spacing.md,
  },
  errorTitle: {
    ...theme.typography.title2,
    color: theme.colors.text.primary,
    marginBottom: theme.spacing.sm,
  },
  retryButton: {
    marginTop: theme.spacing.md,
  },
  inputAccessory: {
    backgroundColor: theme.colors.background.secondary,
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.xs,
    flexDirection: "row",
    justifyContent: "flex-end",
    alignItems: "center",
    borderTopWidth: 0.5,
    borderTopColor: theme.colors.separator.primary,
  },
  accessoryButton: {
    width: 32,
    height: 32,
    borderRadius: 16,
    backgroundColor: theme.colors.brand.primary,
    justifyContent: "center",
    alignItems: "center",
  },
  accessoryButtonDisabled: {
    opacity: 0.3,
  },
  accessoryButtonText: {
    color: "#fff",
    fontSize: 18,
    fontWeight: "600",
  },
});
