import { useState, useEffect, useRef } from "react";
import {
  View,
  Text,
  ScrollView,
  StyleSheet,
  ActivityIndicator,
  Pressable,
  Image,
  Platform,
} from "react-native";
import {
  SafeAreaView,
  useSafeAreaInsets,
} from "react-native-safe-area-context";
import { useRouter } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { BlurView } from "expo-blur";
import { useHeaderHeight } from "@react-navigation/elements";
import { api, CodespaceInfo } from "../lib/api";
import { useAuth } from "../hooks/useAuth";
import { GlassInput, IOSButton } from "../components/ui";
import { theme } from "../theme";

type Phase = "connect" | "connecting" | "setup" | "selection" | "error";

export default function CodespaceScreen() {
  const router = useRouter();
  const { logout } = useAuth();
  const headerHeight = useHeaderHeight();
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

            // Explicitly cleanup before navigation
            if (cleanupRef.current) {
              cleanupRef.current();
            }
            cleanupRef.current = null;
            // Clear connecting phase to stop loading spinner
            setPhase("connect");
            // Navigate to workspaces after successful connection
            setTimeout(() => {
              console.log("🎯 Navigating to workspaces...");
              router.push("/workspaces");
            }, 1000);
          } else if (event.type === "error") {
            console.log("🎯 Error event:", event.message);
            setError(event.message);
            setPhase("error");
            // Cleanup connection on error
            if (cleanupRef.current) {
              cleanupRef.current();
            }
            cleanupRef.current = null;
          } else if (event.type === "setup") {
            console.log("🎯 Setup event:", event.message);
            setPhase("setup");
            setError(event.message);
            // Cleanup connection since we're in setup phase
            if (cleanupRef.current) {
              cleanupRef.current();
            }
            cleanupRef.current = null;
          } else if (event.type === "multiple") {
            console.log("🎯 Multiple codespaces found:", event.codespaces);
            setCodespaces(event.codespaces);
            setPhase("selection");
            // Cleanup connection since we're in selection phase
            if (cleanupRef.current) {
              cleanupRef.current();
            }
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
      // Cleanup connection on error
      if (cleanupRef.current) {
        cleanupRef.current();
      }
      cleanupRef.current = null;
    }
  };

  const handleLogout = async () => {
    await logout();
    router.replace("/auth");
  };

  if (phase === "setup") {
    return (
      <View style={styles.container}>
        <ScrollView
          contentContainerStyle={[
            styles.scrollContent,
            Platform.OS === "ios" && { paddingTop: headerHeight },
          ]}
        >
          <BlurView intensity={100} tint="prominent" style={styles.card}>
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
            <IOSButton
              title="Back"
              onPress={() => setPhase("connect")}
              variant="secondary"
            />
          </BlurView>
        </ScrollView>
      </View>
    );
  }

  if (phase === "selection") {
    return (
      <View style={styles.container}>
        <ScrollView
          contentContainerStyle={[
            styles.scrollContent,
            Platform.OS === "ios" && { paddingTop: headerHeight },
          ]}
        >
          <BlurView intensity={100} tint="prominent" style={styles.card}>
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
            <IOSButton
              title="Back"
              onPress={() => setPhase("connect")}
              variant="secondary"
            />
          </BlurView>
        </ScrollView>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={[
          styles.scrollContent,
          Platform.OS === "ios" && { paddingTop: headerHeight },
        ]}
      >
        <BlurView intensity={100} tint="prominent" style={styles.card}>
          <View style={styles.logoContainer}>
            <Image
              source={require("../assets/logo.png")}
              style={styles.logo}
              resizeMode="contain"
            />
          </View>
          <Text style={styles.title}>Catnip</Text>
          <Text style={styles.subtitle}>
            {orgName
              ? `Access codespaces in ${orgName}`
              : "Access your GitHub Codespaces"}
          </Text>

          <IOSButton
            title={
              phase === "connecting" ? "Connecting..." : "Access My Codespace"
            }
            onPress={() => handleConnect(undefined, orgName || undefined)}
            disabled={phase === "connecting"}
            loading={phase === "connecting"}
            variant="primary"
            size="large"
            style={styles.primaryButton}
          />

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
            <GlassInput
              placeholder="Organization name (e.g., wandb)"
              value={orgName}
              onChangeText={setOrgName}
              editable={phase !== "connecting"}
              containerStyle={styles.input}
            />
            <IOSButton
              title="Go"
              onPress={() => handleConnect(undefined, orgName)}
              disabled={!orgName || phase === "connecting"}
              variant="secondary"
              size="medium"
              style={styles.goButton}
            />
          </View>

          <IOSButton
            title="Logout"
            onPress={handleLogout}
            variant="tertiary"
            size="small"
            style={styles.logoutButton}
          />
        </BlurView>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.colors.background.grouped,
  },
  scrollContent: {
    flexGrow: 1,
    padding: theme.spacing.lg,
    paddingBottom: theme.spacing.xl,
  },
  card: {
    borderRadius: theme.spacing.radius.xl,
    padding: theme.spacing.component.cardPadding,
    overflow: "hidden",
    ...theme.shadows.lg,
  },
  logoContainer: {
    alignItems: "center",
    marginBottom: theme.spacing.md,
  },
  logo: {
    width: 80,
    height: 80,
  },
  title: {
    ...theme.typography.largeTitle,
    color: theme.colors.text.primary,
    textAlign: "center",
    marginBottom: theme.spacing.sm,
  },
  subtitle: {
    ...theme.typography.body,
    color: theme.colors.text.secondary,
    textAlign: "center",
    marginBottom: theme.spacing.lg,
  },
  primaryButton: {
    marginBottom: theme.spacing.md,
  },
  statusBox: {
    backgroundColor: "rgba(0, 122, 255, 0.12)",
    borderWidth: 1,
    borderColor: "rgba(0, 122, 255, 0.25)",
    borderRadius: theme.spacing.radius.md,
    padding: theme.spacing.md,
    marginBottom: theme.spacing.md,
  },
  statusText: {
    color: "#007AFF",
    fontSize: 14,
    textAlign: "center",
    fontWeight: "600",
  },
  errorBox: {
    backgroundColor: "rgba(255, 59, 48, 0.12)",
    borderWidth: 1,
    borderColor: "rgba(255, 59, 48, 0.25)",
    borderRadius: theme.spacing.radius.md,
    padding: theme.spacing.md,
    marginBottom: theme.spacing.md,
  },
  errorText: {
    color: "#FF3B30",
    fontSize: 14,
    textAlign: "center",
    fontWeight: "600",
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: theme.colors.separator.primary,
    marginVertical: theme.spacing.lg,
    opacity: 0.5,
  },
  orText: {
    ...theme.typography.subheadline,
    color: theme.colors.text.secondary,
    marginBottom: theme.spacing.md,
  },
  inputContainer: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    marginBottom: theme.spacing.lg,
  },
  input: {
    flex: 1,
    minWidth: 0,
  },
  goButton: {
    minWidth: 80,
    flexShrink: 0,
  },
  logoutButton: {
    alignItems: "center",
    marginTop: theme.spacing.md,
  },
  cardTitle: {
    ...theme.typography.title2,
    color: theme.colors.text.primary,
    marginBottom: theme.spacing.sm,
  },
  description: {
    ...theme.typography.body,
    color: theme.colors.text.secondary,
    marginBottom: theme.spacing.lg,
  },
  setupSteps: {
    marginBottom: theme.spacing.lg,
  },
  stepText: {
    ...theme.typography.callout,
    color: theme.colors.text.primary,
    marginBottom: theme.spacing.md,
  },
  codeBlock: {
    backgroundColor: "rgba(0, 0, 0, 0.2)",
    borderWidth: 1,
    borderColor: "rgba(255, 255, 255, 0.05)",
    borderRadius: theme.spacing.radius.md,
    padding: theme.spacing.md,
    marginVertical: theme.spacing.sm,
  },
  codeText: {
    ...theme.typography.codeRegular,
    color: theme.colors.status.success,
  },
  backButton: {
    marginTop: theme.spacing.md,
  },
  codespaceItem: {
    backgroundColor: "rgba(255, 255, 255, 0.05)",
    borderWidth: 1,
    borderColor: "rgba(255, 255, 255, 0.1)",
    borderRadius: theme.spacing.radius.md,
    padding: theme.spacing.md,
    marginBottom: theme.spacing.md,
  },
  codespaceTitle: {
    ...theme.typography.headline,
    color: theme.colors.text.primary,
    marginBottom: theme.spacing.xs,
  },
  codespaceRepo: {
    ...theme.typography.callout,
    color: theme.colors.brand.accent,
    marginBottom: theme.spacing.xs,
  },
  codespaceDate: {
    ...theme.typography.caption1,
    color: theme.colors.text.tertiary,
  },
});
