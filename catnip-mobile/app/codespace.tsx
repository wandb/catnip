import { useState, useEffect, useRef } from "react";
import {
  View,
  Text,
  Pressable,
  TextInput,
  ScrollView,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useRouter } from "expo-router";
import { LinearGradient } from "expo-linear-gradient";
import * as SecureStore from "expo-secure-store";
import { api, CodespaceInfo } from "../lib/api";
import { useAuth } from "../hooks/useAuth";

type Phase = "connect" | "connecting" | "setup" | "selection" | "error";

export default function CodespaceScreen() {
  const router = useRouter();
  const { logout } = useAuth();
  const [phase, setPhase] = useState<Phase>("connect");
  const [orgName, setOrgName] = useState("");
  const [statusMessage, setStatusMessage] = useState("");
  const [error, setError] = useState("");
  const [codespaces, setCodespaces] = useState<CodespaceInfo[]>([]);
  const cleanupRef = useRef<(() => void) | null>(null);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (cleanupRef.current) {
        cleanupRef.current();
        cleanupRef.current = null;
      }
    };
  }, []);

  const handleConnect = async (codespaceName?: string, org?: string) => {
    console.log("🎯 handleConnect called with:", { codespaceName, org });

    // Cleanup any existing connection
    if (cleanupRef.current) {
      cleanupRef.current();
      cleanupRef.current = null;
    }

    setPhase("connecting");
    setError("");
    setStatusMessage("🔄 Finding your codespace...");

    try {
      console.log("🎯 Calling api.connectCodespace...");
      const { promise, cleanup } = api.connectCodespace(
        codespaceName,
        org,
        async (event) => {
          console.log("🎯 Received event:", event);

          if (event.type === "status") {
            console.log("🎯 Status event:", event.message);
            setStatusMessage(event.message);
          } else if (event.type === "success") {
            console.log("🎯 Success event:", event.message);
            setStatusMessage("✅ " + event.message);

            // Extract and store codespace name from the success event
            if (event.codespaceUrl) {
              try {
                const url = new URL(event.codespaceUrl);
                const hostname = url.hostname;
                // Extract codespace name from hostname like "codespace-name-6369.app.github.dev"
                const codespaceNameMatch = hostname.match(
                  /^(.+)-6369\.app\.github\.dev$/,
                );
                if (codespaceNameMatch) {
                  const extractedCodespaceName = codespaceNameMatch[1];
                  console.log(
                    "🎯 Extracted codespace name:",
                    extractedCodespaceName,
                  );
                  await SecureStore.setItemAsync(
                    "codespace_name",
                    extractedCodespaceName,
                  );
                }
              } catch (error) {
                console.error("🎯 Failed to extract codespace name:", error);
              }
            }

            // Clear cleanup ref since connection succeeded
            cleanupRef.current = null;
            // Navigate to workspaces after successful connection
            setTimeout(() => {
              console.log("🎯 Navigating to workspaces...");
              router.replace("/workspaces");
            }, 1000);
          } else if (event.type === "error") {
            console.log("🎯 Error event:", event.message);
            setError(event.message);
            setPhase("error");
            // Clear cleanup ref since connection failed
            cleanupRef.current = null;
          } else if (event.type === "setup") {
            console.log("🎯 Setup event:", event.message);
            setPhase("setup");
            setError(event.message);
            // Clear cleanup ref since we're in setup phase
            cleanupRef.current = null;
          } else if (event.type === "multiple") {
            console.log("🎯 Multiple codespaces found:", event.codespaces);
            setCodespaces(event.codespaces);
            setPhase("selection");
            // Clear cleanup ref since we're in selection phase
            cleanupRef.current = null;
          }
        },
      );

      // Store cleanup function
      cleanupRef.current = cleanup;

      await promise;
      console.log("🎯 api.connectCodespace completed successfully");
    } catch (err: any) {
      console.error("🎯 Error in handleConnect:", err);
      setError(err.message || "Connection failed");
      setPhase("error");
      // Clear cleanup ref on error
      cleanupRef.current = null;
    }
  };

  const handleLogout = async () => {
    await logout();
    router.replace("/auth");
  };

  if (phase === "setup") {
    return (
      <SafeAreaView style={styles.container}>
        <ScrollView contentContainerStyle={styles.scrollContent}>
          <View style={styles.card}>
            <Text style={styles.cardTitle}>⚠️ Setup Required</Text>
            <Text style={styles.description}>
              No Catnip codespaces found. To use Catnip:
            </Text>
            <View style={styles.setupSteps}>
              <Text style={styles.stepText}>
                1. Add to your .devcontainer/devcontainer.json:
              </Text>
              <View style={styles.codeBlock}>
                <Text style={styles.codeText}>
                  {`"features": {
  "ghcr.io/wandb/catnip/feature:1": {}
}`}
                </Text>
              </View>
              <Text style={styles.stepText}>2. Create a new codespace</Text>
              <Text style={styles.stepText}>3. Return here to connect</Text>
            </View>
            <Pressable
              onPress={() => setPhase("connect")}
              style={styles.secondaryButton}
            >
              <Text style={styles.secondaryButtonText}>Back</Text>
            </Pressable>
          </View>
        </ScrollView>
      </SafeAreaView>
    );
  }

  if (phase === "selection") {
    return (
      <SafeAreaView style={styles.container}>
        <ScrollView contentContainerStyle={styles.scrollContent}>
          <View style={styles.card}>
            <Text style={styles.cardTitle}>Select Codespace</Text>
            <Text style={styles.description}>Multiple codespaces found:</Text>
            {codespaces.map((cs, index) => (
              <Pressable
                key={index}
                style={styles.codespaceItem}
                onPress={() => handleConnect(cs.name, orgName)}
              >
                <Text style={styles.codespaceTitle}>
                  {cs.name.replace(/-/g, " ")}
                </Text>
                {cs.repository && (
                  <Text style={styles.codespaceRepo}>{cs.repository}</Text>
                )}
                <Text style={styles.codespaceDate}>
                  Last used: {new Date(cs.lastUsed).toLocaleDateString()}
                </Text>
              </Pressable>
            ))}
            <Pressable
              onPress={() => setPhase("connect")}
              style={styles.secondaryButton}
            >
              <Text style={styles.secondaryButtonText}>Back</Text>
            </Pressable>
          </View>
        </ScrollView>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={styles.scrollContent}>
        <View style={styles.card}>
          <Text style={styles.logo}>🐱</Text>
          <Text style={styles.title}>Catnip</Text>
          <Text style={styles.subtitle}>
            {orgName
              ? `Access codespaces in ${orgName}`
              : "Access your GitHub Codespaces"}
          </Text>

          <Pressable
            onPress={() => handleConnect(undefined, orgName || undefined)}
            disabled={phase === "connecting"}
          >
            <LinearGradient
              colors={["#7c3aed", "#3b82f6"]}
              start={{ x: 0, y: 0 }}
              end={{ x: 1, y: 0 }}
              style={[
                styles.button,
                phase === "connecting" && styles.buttonDisabled,
              ]}
            >
              {phase === "connecting" ? (
                <View style={styles.buttonContent}>
                  <ActivityIndicator color="#fff" size="small" />
                  <Text style={styles.buttonText}>Connecting...</Text>
                </View>
              ) : (
                <Text style={styles.buttonText}>Access My Codespace</Text>
              )}
            </LinearGradient>
          </Pressable>

          {statusMessage ? (
            <View style={styles.statusBox}>
              <Text style={styles.statusText}>{statusMessage}</Text>
            </View>
          ) : null}

          {error ? (
            <View style={styles.errorBox}>
              <Text style={styles.errorText}>{error}</Text>
            </View>
          ) : null}

          <View style={styles.divider} />

          <Text style={styles.orText}>Or access a specific organization:</Text>
          <View style={styles.inputContainer}>
            <TextInput
              style={styles.input}
              placeholder="Organization name (e.g., wandb)"
              placeholderTextColor="#666"
              value={orgName}
              onChangeText={setOrgName}
              editable={phase !== "connecting"}
            />
            <Pressable
              style={[
                styles.goButton,
                (!orgName || phase === "connecting") && styles.buttonDisabled,
              ]}
              onPress={() => handleConnect(undefined, orgName)}
              disabled={!orgName || phase === "connecting"}
            >
              <Text style={styles.goButtonText}>Go</Text>
            </Pressable>
          </View>

          <Pressable onPress={handleLogout} style={styles.logoutButton}>
            <Text style={styles.logoutText}>Logout</Text>
          </Pressable>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#0a0a0a",
  },
  scrollContent: {
    flexGrow: 1,
    justifyContent: "center",
    padding: 24,
  },
  card: {
    backgroundColor: "#1a1a1a",
    borderRadius: 16,
    padding: 24,
    borderWidth: 1,
    borderColor: "#333",
  },
  logo: {
    fontSize: 48,
    textAlign: "center",
    marginBottom: 16,
  },
  title: {
    fontSize: 28,
    fontWeight: "bold",
    color: "#fff",
    textAlign: "center",
    marginBottom: 8,
  },
  subtitle: {
    fontSize: 14,
    color: "#999",
    textAlign: "center",
    marginBottom: 24,
  },
  button: {
    paddingVertical: 14,
    borderRadius: 12,
    alignItems: "center",
    marginBottom: 16,
  },
  buttonDisabled: {
    opacity: 0.5,
  },
  buttonContent: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  buttonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  statusBox: {
    backgroundColor: "rgba(59, 130, 246, 0.1)",
    borderWidth: 1,
    borderColor: "rgba(59, 130, 246, 0.3)",
    borderRadius: 12,
    padding: 12,
    marginBottom: 16,
  },
  statusText: {
    color: "#93bbfc",
    fontSize: 14,
    textAlign: "center",
  },
  errorBox: {
    backgroundColor: "rgba(239, 68, 68, 0.1)",
    borderWidth: 1,
    borderColor: "rgba(239, 68, 68, 0.3)",
    borderRadius: 12,
    padding: 12,
    marginBottom: 16,
  },
  errorText: {
    color: "#fca5a5",
    fontSize: 14,
    textAlign: "center",
  },
  divider: {
    height: 1,
    backgroundColor: "#333",
    marginVertical: 24,
  },
  orText: {
    color: "#999",
    fontSize: 14,
    marginBottom: 16,
  },
  inputContainer: {
    flexDirection: "row",
    gap: 8,
    marginBottom: 24,
  },
  input: {
    flex: 1,
    backgroundColor: "#0a0a0a",
    borderWidth: 1,
    borderColor: "#333",
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12,
    color: "#fff",
    fontSize: 14,
  },
  goButton: {
    backgroundColor: "#4b5563",
    paddingHorizontal: 24,
    borderRadius: 12,
    justifyContent: "center",
  },
  goButtonText: {
    color: "#fff",
    fontSize: 14,
    fontWeight: "600",
  },
  logoutButton: {
    alignItems: "center",
  },
  logoutText: {
    color: "#ef4444",
    fontSize: 14,
  },
  cardTitle: {
    fontSize: 20,
    fontWeight: "bold",
    color: "#fff",
    marginBottom: 8,
  },
  description: {
    fontSize: 14,
    color: "#999",
    marginBottom: 24,
  },
  setupSteps: {
    marginBottom: 24,
  },
  stepText: {
    color: "#ccc",
    fontSize: 14,
    marginBottom: 12,
  },
  codeBlock: {
    backgroundColor: "#0a0a0a",
    borderRadius: 8,
    padding: 12,
    marginVertical: 8,
  },
  codeText: {
    color: "#4ade80",
    fontFamily: "monospace",
    fontSize: 12,
  },
  secondaryButton: {
    backgroundColor: "#333",
    paddingVertical: 14,
    borderRadius: 12,
    alignItems: "center",
  },
  secondaryButtonText: {
    color: "#ccc",
    fontSize: 16,
    fontWeight: "600",
  },
  codespaceItem: {
    backgroundColor: "#0a0a0a",
    borderRadius: 12,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#333",
  },
  codespaceTitle: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
    marginBottom: 4,
  },
  codespaceRepo: {
    color: "#3b82f6",
    fontSize: 14,
    marginBottom: 4,
  },
  codespaceDate: {
    color: "#666",
    fontSize: 12,
  },
});
