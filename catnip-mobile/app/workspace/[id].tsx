import { useState, useEffect, useCallback } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  ScrollView,
  KeyboardAvoidingView,
  Platform,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useNavigation } from "expo-router";
import { LinearGradient } from "expo-linear-gradient";
import { api, WorkspaceInfo, Todo } from "../../lib/api";

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
  const navigation = useNavigation();

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

      // Determine phase based on workspace state
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
    if (phase !== "working") return;

    const interval = setInterval(async () => {
      try {
        const decodedId = decodeURIComponent(id);
        const data = await api.getWorkspace(decodedId);
        setWorkspace(data);

        // Check if work is completed
        if (
          data.claude_activity_state === "inactive" ||
          (data.todos && data.todos.every((t) => t.status === "completed"))
        ) {
          setPhase("completed");
        }
      } catch (err) {
        console.error("Failed to poll workspace:", err);
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [phase, id]);

  // Set navigation title
  useEffect(() => {
    if (workspace) {
      const title = workspace.name.split("/")[1] || workspace.name;
      navigation.setOptions({ title });
    }
  }, [workspace, navigation]);

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
          <Pressable onPress={loadWorkspace} style={styles.retryButton}>
            <Text style={styles.retryButtonText}>Retry</Text>
          </Pressable>
        </View>
      </SafeAreaView>
    );
  }

  const cleanBranch = workspace.branch.startsWith("/")
    ? workspace.branch.slice(1)
    : workspace.branch;

  return (
    <SafeAreaView style={styles.container} edges={["bottom"]}>
      <KeyboardAvoidingView
        style={styles.container}
        behavior={Platform.OS === "ios" ? "padding" : "height"}
      >
        <View style={styles.header}>
          <Text style={styles.headerTitle}>
            {workspace.name.split("/")[1] || workspace.name}
          </Text>
          <Text style={styles.headerSubtitle}>
            {workspace.repo_id || "Unknown repo"} · {cleanBranch}
          </Text>
        </View>

        <ScrollView
          style={styles.content}
          contentContainerStyle={styles.contentContainer}
          keyboardShouldPersistTaps="handled"
        >
          {phase === "input" && (
            <View style={styles.inputSection}>
              <Text style={styles.sectionTitle}>Start Working</Text>
              <Text style={styles.sectionSubtitle}>
                Describe what you'd like to work on
              </Text>
              <TextInput
                style={styles.promptInput}
                placeholder="Describe your task..."
                placeholderTextColor="#666"
                value={prompt}
                onChangeText={setPrompt}
                multiline
                textAlignVertical="top"
              />
            </View>
          )}

          {phase === "working" && (
            <View style={styles.workingSection}>
              <View style={styles.statusContainer}>
                <ActivityIndicator size="small" color="#7c3aed" />
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
            <Pressable
              onPress={handleSendPrompt}
              disabled={!prompt.trim() || isSubmitting || !workspace}
            >
              <LinearGradient
                colors={["#7c3aed", "#3b82f6"]}
                start={{ x: 0, y: 0 }}
                end={{ x: 1, y: 0 }}
                style={[
                  styles.primaryButton,
                  (!prompt.trim() || isSubmitting || !workspace) &&
                    styles.buttonDisabled,
                ]}
              >
                {isSubmitting ? (
                  <ActivityIndicator color="#fff" />
                ) : (
                  <Text style={styles.primaryButtonText}>Start Working</Text>
                )}
              </LinearGradient>
            </Pressable>
          )}

          {phase === "completed" && (
            <>
              {showPromptInput ? (
                <View style={styles.promptInputContainer}>
                  <TextInput
                    style={styles.bottomPromptInput}
                    placeholder="Describe what you'd like to change..."
                    placeholderTextColor="#666"
                    value={prompt}
                    onChangeText={setPrompt}
                    multiline
                    textAlignVertical="top"
                  />
                  <View style={styles.buttonRow}>
                    <Pressable
                      style={[styles.primaryButton, styles.flexButton]}
                      onPress={handleSendPrompt}
                      disabled={!prompt.trim() || isSubmitting || !workspace}
                    >
                      <Text style={styles.primaryButtonText}>Send</Text>
                    </Pressable>
                    <Pressable
                      style={[styles.secondaryButton, styles.flexButton]}
                      onPress={() => {
                        setShowPromptInput(false);
                        setPrompt("");
                      }}
                    >
                      <Text style={styles.secondaryButtonText}>Cancel</Text>
                    </Pressable>
                  </View>
                </View>
              ) : (
                <Pressable
                  style={styles.primaryButton}
                  onPress={() => setShowPromptInput(true)}
                >
                  <Text style={styles.primaryButtonText}>Ask for changes</Text>
                </Pressable>
              )}
            </>
          )}
        </View>
      </KeyboardAvoidingView>
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
    fontSize: 20,
    fontWeight: "600",
    color: "#fff",
    marginBottom: 4,
  },
  headerSubtitle: {
    fontSize: 14,
    color: "#666",
  },
  content: {
    flex: 1,
  },
  contentContainer: {
    padding: 20,
  },
  inputSection: {
    alignItems: "center",
    marginTop: 40,
  },
  sectionTitle: {
    fontSize: 24,
    fontWeight: "600",
    color: "#fff",
    marginBottom: 8,
  },
  sectionSubtitle: {
    fontSize: 14,
    color: "#666",
    marginBottom: 24,
    textAlign: "center",
  },
  promptInput: {
    width: "100%",
    backgroundColor: "#1a1a1a",
    borderWidth: 1,
    borderColor: "#333",
    borderRadius: 12,
    padding: 16,
    color: "#fff",
    fontSize: 14,
    minHeight: 120,
  },
  workingSection: {
    marginTop: 20,
  },
  statusContainer: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    marginBottom: 24,
  },
  statusText: {
    color: "#999",
    fontSize: 14,
  },
  messageBox: {
    backgroundColor: "rgba(124, 58, 237, 0.1)",
    borderWidth: 1,
    borderColor: "rgba(124, 58, 237, 0.2)",
    borderRadius: 12,
    padding: 16,
    marginBottom: 24,
  },
  messageLabel: {
    fontSize: 12,
    color: "rgba(124, 58, 237, 0.8)",
    marginBottom: 8,
    fontWeight: "600",
  },
  messageText: {
    color: "#ccc",
    fontSize: 14,
    lineHeight: 20,
  },
  completedSection: {
    marginTop: 20,
  },
  sectionLabel: {
    fontSize: 14,
    color: "#999",
    marginBottom: 12,
    fontWeight: "600",
  },
  todosContainer: {
    gap: 8,
  },
  todoItem: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 12,
    paddingVertical: 8,
  },
  todoStatus: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: "#333",
    marginTop: 6,
  },
  todoCompleted: {
    backgroundColor: "#22c55e",
  },
  todoInProgress: {
    backgroundColor: "#eab308",
  },
  todoText: {
    color: "#ccc",
    fontSize: 14,
    flex: 1,
    lineHeight: 20,
  },
  footer: {
    padding: 20,
    borderTopWidth: 1,
    borderTopColor: "#1a1a1a",
  },
  primaryButton: {
    backgroundColor: "#7c3aed",
    paddingVertical: 14,
    borderRadius: 12,
    alignItems: "center",
  },
  secondaryButton: {
    backgroundColor: "#333",
    paddingVertical: 14,
    borderRadius: 12,
    alignItems: "center",
  },
  buttonDisabled: {
    opacity: 0.5,
  },
  primaryButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  secondaryButtonText: {
    color: "#ccc",
    fontSize: 16,
    fontWeight: "600",
  },
  promptInputContainer: {
    gap: 12,
  },
  bottomPromptInput: {
    backgroundColor: "#1a1a1a",
    borderWidth: 1,
    borderColor: "#333",
    borderRadius: 12,
    padding: 12,
    color: "#fff",
    fontSize: 14,
    minHeight: 80,
  },
  buttonRow: {
    flexDirection: "row",
    gap: 12,
  },
  flexButton: {
    flex: 1,
  },
  errorBox: {
    backgroundColor: "rgba(239, 68, 68, 0.1)",
    borderWidth: 1,
    borderColor: "rgba(239, 68, 68, 0.3)",
    borderRadius: 12,
    padding: 12,
    marginTop: 16,
  },
  errorText: {
    color: "#fca5a5",
    fontSize: 14,
  },
  centerContainer: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: 20,
  },
  loadingText: {
    color: "#666",
    marginTop: 16,
    fontSize: 16,
  },
  errorTitle: {
    fontSize: 20,
    fontWeight: "600",
    color: "#fff",
    marginBottom: 8,
  },
  retryButton: {
    backgroundColor: "#7c3aed",
    paddingVertical: 12,
    paddingHorizontal: 24,
    borderRadius: 12,
    marginTop: 16,
  },
  retryButtonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
});
