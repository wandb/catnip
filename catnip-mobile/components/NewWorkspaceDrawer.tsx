import React, {
  useState,
  useRef,
  useMemo,
  useCallback,
  useEffect,
} from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  Alert,
  Keyboard,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  TextInput,
  ActivityIndicator,
} from "react-native";
import BottomSheet, {
  BottomSheetView,
  BottomSheetScrollView,
} from "@gorhom/bottom-sheet";
import { IOSButton } from "./ui";
import { theme } from "../theme";
import { Ionicons } from "@expo/vector-icons";
import { api } from "../lib/api";

interface Repository {
  id: string;
  name: string;
  owner: string;
  full_name: string;
  html_url: string;
  default_branch: string;
}

interface NewWorkspaceDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onCreateWorkspace: (repository: string, branch: string) => Promise<void>;
  existingWorkspaces?: Array<{ repo_id: string; branch: string }>;
}

const DropdownSelector = ({
  label,
  value,
  onSelect,
  options,
  placeholder,
  loading = false,
}: {
  label: string;
  value: string;
  onSelect: (value: string) => void;
  options: Array<{ label: string; value: string }>;
  placeholder: string;
  loading?: boolean;
}) => {
  const [isOpen, setIsOpen] = useState(false);

  console.log("🐱 DropdownSelector render:", {
    label,
    value,
    optionsCount: options.length,
    isOpen,
  });

  return (
    <View style={styles.dropdownContainer}>
      <Text style={styles.inputLabel}>{label}</Text>
      <TouchableOpacity
        style={styles.dropdown}
        onPress={() => setIsOpen(!isOpen)}
        disabled={loading}
      >
        <Text
          style={[styles.dropdownText, !value && styles.dropdownPlaceholder]}
        >
          {value || placeholder}
        </Text>
        {loading ? (
          <ActivityIndicator size="small" color={theme.colors.text.secondary} />
        ) : (
          <Ionicons
            name={isOpen ? "chevron-up" : "chevron-down"}
            size={20}
            color={theme.colors.text.secondary}
          />
        )}
      </TouchableOpacity>

      {isOpen && (
        <View style={styles.dropdownOptions}>
          <ScrollView
            style={styles.optionsScroll}
            showsVerticalScrollIndicator={false}
          >
            {options.length > 0 ? (
              options.map((option) => (
                <TouchableOpacity
                  key={option.value}
                  style={styles.option}
                  onPress={() => {
                    onSelect(option.value);
                    setIsOpen(false);
                  }}
                >
                  <Text
                    style={[
                      styles.optionText,
                      value === option.value && styles.selectedOptionText,
                    ]}
                  >
                    {option.label}
                  </Text>
                  {value === option.value && (
                    <Ionicons
                      name="checkmark"
                      size={20}
                      color={theme.colors.brand.primary}
                    />
                  )}
                </TouchableOpacity>
              ))
            ) : (
              <TouchableOpacity style={styles.option}>
                <Text style={styles.optionText}>No options available</Text>
              </TouchableOpacity>
            )}
          </ScrollView>
        </View>
      )}
    </View>
  );
};

export function NewWorkspaceDrawer({
  isOpen,
  onClose,
  onCreateWorkspace,
  existingWorkspaces = [],
}: NewWorkspaceDrawerProps) {
  const bottomSheetRef = useRef<BottomSheet>(null);
  const [selectedRepo, setSelectedRepo] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("");
  const [isCreating, setIsCreating] = useState(false);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [branches, setBranches] = useState<string[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [loadingBranches, setLoadingBranches] = useState(false);
  const [keyboardHeight, setKeyboardHeight] = useState(0);

  const snapPoints = useMemo(() => {
    const baseHeight = keyboardHeight > 0 ? "85%" : "60%";
    return [baseHeight];
  }, [keyboardHeight]);

  const handleSheetChanges = useCallback(
    (index: number) => {
      if (index === -1) {
        onClose();
      }
    },
    [onClose],
  );

  // Handle keyboard events
  useEffect(() => {
    const keyboardWillShow = Keyboard.addListener(
      Platform.OS === "ios" ? "keyboardWillShow" : "keyboardDidShow",
      (e) => {
        setKeyboardHeight(e.endCoordinates.height);
      },
    );

    const keyboardWillHide = Keyboard.addListener(
      Platform.OS === "ios" ? "keyboardWillHide" : "keyboardDidHide",
      () => {
        setKeyboardHeight(0);
      },
    );

    return () => {
      keyboardWillShow.remove();
      keyboardWillHide.remove();
    };
  }, []);

  // Load repositories when drawer opens
  useEffect(() => {
    if (isOpen) {
      loadRepositories();
    }
  }, [isOpen]);

  const loadRepositories = async () => {
    setLoadingRepos(true);
    try {
      // Get unique repositories from existing workspaces
      const existingRepos = [
        ...new Set(existingWorkspaces.map((w) => w.repo_id)),
      ];

      // Mock repositories based on existing workspaces plus some defaults
      const mockRepos: Repository[] = [
        ...existingRepos.map((repoId) => ({
          id: repoId,
          name: repoId.split("/")[1] || repoId,
          owner: repoId.split("/")[0] || "unknown",
          full_name: repoId,
          html_url: `https://github.com/${repoId}`,
          default_branch: "main",
        })),
        {
          id: "catnip-run/catnip",
          name: "catnip",
          owner: "catnip-run",
          full_name: "catnip-run/catnip",
          html_url: "https://github.com/catnip-run/catnip",
          default_branch: "main",
        },
        {
          id: "microsoft/vscode",
          name: "vscode",
          owner: "microsoft",
          full_name: "microsoft/vscode",
          html_url: "https://github.com/microsoft/vscode",
          default_branch: "main",
        },
      ];

      // Remove duplicates
      const uniqueRepos = mockRepos.filter(
        (repo, index, self) =>
          index === self.findIndex((r) => r.full_name === repo.full_name),
      );

      console.log(
        "🐱 Loading repositories with existing workspaces:",
        uniqueRepos,
      );
      setRepositories(uniqueRepos);

      // Auto-select the most recent repository if available
      if (existingRepos.length > 0 && !selectedRepo) {
        const mostRecentRepo = existingRepos[0];
        setSelectedRepo(mostRecentRepo);
        loadBranches(mostRecentRepo);
      }
    } catch (error) {
      console.error("Failed to load repositories:", error);
      Alert.alert("Error", "Failed to load repositories");
    } finally {
      setLoadingRepos(false);
    }
  };

  const loadBranches = async (repoId: string) => {
    console.log("🐱 Loading branches for repo:", repoId);
    setLoadingBranches(true);
    try {
      // Mock branches for now since the API doesn't provide this endpoint
      const mockBranches = [
        "main",
        "develop",
        "feature/mobile-app",
        "bugfix/auth-flow",
      ];
      console.log("🐱 Setting branches:", mockBranches);
      setBranches(mockBranches);

      // Auto-select default branch if available
      const repo = repositories.find((r) => r.full_name === repoId);
      if (repo?.default_branch && mockBranches.includes(repo.default_branch)) {
        setSelectedBranch(repo.default_branch);
      } else if (mockBranches.length > 0) {
        setSelectedBranch(mockBranches[0]);
      }
    } catch (error) {
      console.error("Failed to load branches:", error);
      Alert.alert("Error", "Failed to load branches");
      setBranches([]);
    } finally {
      setLoadingBranches(false);
    }
  };

  React.useEffect(() => {
    if (isOpen) {
      bottomSheetRef.current?.expand();
    } else {
      bottomSheetRef.current?.close();
    }
  }, [isOpen]);

  const handleRepoSelect = (repoFullName: string) => {
    setSelectedRepo(repoFullName);
    setSelectedBranch(""); // Reset branch selection
    setBranches([]); // Clear branches
    if (repoFullName) {
      loadBranches(repoFullName);
    }
  };

  const handleCreate = async () => {
    if (!selectedRepo.trim()) {
      Alert.alert("Error", "Please select a repository");
      return;
    }

    if (!selectedBranch.trim()) {
      Alert.alert("Error", "Please select a branch");
      return;
    }

    setIsCreating(true);
    try {
      await onCreateWorkspace(selectedRepo.trim(), selectedBranch.trim());

      // Reset form and close
      setSelectedRepo("");
      setSelectedBranch("");
      setBranches([]);
      onClose();

      Alert.alert("Success", "Workspace created successfully!");
    } catch (error) {
      console.error("Failed to create workspace:", error);
      Alert.alert(
        "Error",
        `Failed to create workspace: ${
          error instanceof Error ? error.message : "Unknown error"
        }`,
      );
    } finally {
      setIsCreating(false);
    }
  };

  const handleClose = () => {
    Keyboard.dismiss();
    onClose();
  };

  const repoOptions = repositories.map((repo) => ({
    label: repo.full_name,
    value: repo.full_name,
  }));

  const branchOptions = branches.map((branch) => ({
    label: branch,
    value: branch,
  }));

  console.log("🐱 repoOptions:", repoOptions.length, repoOptions);
  console.log("🐱 branchOptions:", branchOptions.length, branchOptions);

  return (
    <BottomSheet
      ref={bottomSheetRef}
      index={isOpen ? 0 : -1}
      snapPoints={snapPoints}
      onChange={handleSheetChanges}
      enablePanDownToClose
      backgroundStyle={styles.bottomSheetBackground}
      handleIndicatorStyle={styles.handleIndicator}
      keyboardBehavior="interactive"
      keyboardBlurBehavior="restore"
      android_keyboardInputMode="adjustResize"
    >
      <BottomSheetView style={styles.contentContainer}>
        <View style={styles.header}>
          <View style={styles.headerLeft} />
          <Text style={styles.title}>Create Workspace</Text>
          <TouchableOpacity style={styles.closeButton} onPress={handleClose}>
            <Ionicons
              name="close"
              size={24}
              color={theme.colors.text.secondary}
            />
          </TouchableOpacity>
        </View>

        <BottomSheetScrollView
          contentContainerStyle={styles.form}
          keyboardShouldPersistTaps="handled"
          showsVerticalScrollIndicator={false}
        >
          <DropdownSelector
            label="Repository *"
            value={selectedRepo}
            onSelect={handleRepoSelect}
            options={repoOptions}
            placeholder="Select repository"
            loading={loadingRepos}
          />

          <DropdownSelector
            label="Branch *"
            value={selectedBranch}
            onSelect={setSelectedBranch}
            options={branchOptions}
            placeholder="Select branch"
            loading={loadingBranches}
          />

          <View style={styles.buttonContainer}>
            <IOSButton
              title="Cancel"
              onPress={handleClose}
              variant="secondary"
              style={styles.button}
              disabled={isCreating}
            />
            <IOSButton
              title="Create"
              onPress={handleCreate}
              disabled={isCreating || !selectedRepo || !selectedBranch}
              loading={isCreating}
              variant="primary"
              style={styles.button}
            />
          </View>
        </BottomSheetScrollView>
      </BottomSheetView>
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  bottomSheetBackground: {
    backgroundColor: theme.colors.background.secondary,
    borderRadius: theme.spacing.radius.lg,
  },
  handleIndicator: {
    backgroundColor: theme.colors.separator.primary,
    width: 36,
    height: 4,
  },
  contentContainer: {
    flex: 1,
    paddingHorizontal: theme.spacing.md,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.sm,
    marginBottom: theme.spacing.md,
  },
  headerLeft: {
    width: 24,
  },
  title: {
    ...theme.typography.title3,
    color: theme.colors.text.primary,
    fontWeight: "600",
  },
  closeButton: {
    width: 24,
    height: 24,
    justifyContent: "center",
    alignItems: "center",
  },
  form: {
    gap: theme.spacing.lg,
    paddingBottom: theme.spacing.xl,
  },
  dropdownContainer: {
    gap: theme.spacing.sm,
  },
  inputLabel: {
    ...theme.typography.calloutEmphasized,
    color: theme.colors.text.primary,
  },
  dropdown: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
    backgroundColor: theme.colors.fill.tertiary,
    borderRadius: theme.spacing.radius.md,
    borderWidth: 1,
    borderColor: theme.colors.separator.primary,
    minHeight: 44,
  },
  dropdownText: {
    ...theme.typography.body,
    color: theme.colors.text.primary,
    flex: 1,
  },
  dropdownPlaceholder: {
    color: theme.colors.text.tertiary,
  },
  dropdownOptions: {
    backgroundColor: theme.colors.background.secondary,
    borderRadius: theme.spacing.radius.md,
    borderWidth: 1,
    borderColor: theme.colors.separator.primary,
    maxHeight: 150,
    marginTop: 4,
    position: "relative",
    zIndex: 1000,
    ...theme.shadows.sm,
  },
  optionsScroll: {
    flex: 1,
  },
  option: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.xs,
    borderBottomWidth: 1,
    borderBottomColor: theme.colors.separator.primary,
    minHeight: 40,
  },
  optionText: {
    ...theme.typography.body,
    color: theme.colors.text.primary,
    flex: 1,
  },
  selectedOptionText: {
    color: theme.colors.brand.primary,
    fontWeight: "600",
  },
  buttonContainer: {
    flexDirection: "row",
    gap: theme.spacing.md,
    marginTop: theme.spacing.lg,
  },
  button: {
    flex: 1,
  },
});
