import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
  ScrollView,
  TextInput,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useNavigation } from '@react-navigation/native';
import { NativeStackNavigationProp } from '@react-navigation/native-stack';
import * as SecureStore from 'expo-secure-store';
import { RootStackParamList } from '../../App';
import { useAuth } from '../contexts/AuthContext';
import { useApi } from '../contexts/ApiContext';

type NavigationProp = NativeStackNavigationProp<RootStackParamList, 'CodespaceAccess'>;

type StatusStep = 'search' | 'starting' | 'setup' | 'catnip' | 'initializing' | 'health' | 'ready';

interface CodespaceInfo {
  name: string;
  lastUsed: number;
  repository?: string;
}

export default function CodespaceAccessScreen() {
  const navigation = useNavigation<NavigationProp>();
  const { isAuthenticated, logout } = useAuth();
  const { accessCodespace } = useApi();

  const [isConnecting, setIsConnecting] = useState(false);
  const [statusMessage, setStatusMessage] = useState('');
  const [statusStep, setStatusStep] = useState<StatusStep | null>(null);
  const [error, setError] = useState('');
  const [orgName, setOrgName] = useState('');
  const [codespaces, setCodespaces] = useState<CodespaceInfo[]>([]);
  const [showSelection, setShowSelection] = useState(false);
  const [showSetup, setShowSetup] = useState(false);

  const resetState = () => {
    setIsConnecting(false);
    setStatusMessage('');
    setStatusStep(null);
    setError('');
    setCodespaces([]);
    setShowSelection(false);
    setShowSetup(false);
  };

  const connectToCodespace = async (org?: string, codespaceName?: string) => {
    setIsConnecting(true);
    setError('');
    setStatusMessage('🔄 Finding your codespace...');
    setStatusStep('search');

    try {
      const generator = accessCodespace(codespaceName, org);

      for await (const event of generator) {
        if (event.type === 'status') {
          setStatusMessage(event.message);
          setStatusStep(event.step);
        } else if (event.type === 'success') {
          setStatusMessage('✅ ' + event.message);
          setStatusStep('ready');

          // Store the codespace token if provided
          if (event.token) {
            await SecureStore.setItemAsync('codespace_token', event.token);
          }

          // Navigate to workspace list
          setTimeout(() => {
            resetState();
            navigation.replace('WorkspaceList');
          }, 1000);
        } else if (event.type === 'error') {
          setError(event.message);
          setIsConnecting(false);
        } else if (event.type === 'setup') {
          setShowSetup(true);
          setError(event.message);
          setIsConnecting(false);
        } else if (event.type === 'multiple') {
          setCodespaces(event.codespaces);
          setShowSelection(true);
          setIsConnecting(false);
        }
      }
    } catch (err) {
      setError('Connection failed. Please try again.');
      setIsConnecting(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    navigation.replace('Auth');
  };

  if (showSetup) {
    return (
      <SafeAreaView style={styles.container}>
        <ScrollView contentContainerStyle={styles.scrollContent}>
          <View style={styles.card}>
            <Text style={styles.cardTitle}>⚠️ Setup Required</Text>
            <Text style={styles.cardDescription}>
              No Catnip codespaces found. To use Catnip, you need to:
            </Text>
            <View style={styles.setupSteps}>
              <Text style={styles.stepText}>
                1. Add this to your .devcontainer/devcontainer.json:
              </Text>
              <View style={styles.codeBlock}>
                <Text style={styles.codeText}>
                  {`"features": {\n  "ghcr.io/wandb/catnip/feature:1": {}\n}`}
                </Text>
              </View>
              <Text style={styles.stepText}>2. Create a new codespace from your repository</Text>
              <Text style={styles.stepText}>3. Return here to access your codespace</Text>
            </View>
            <TouchableOpacity style={styles.button} onPress={resetState}>
              <Text style={styles.buttonText}>Back</Text>
            </TouchableOpacity>
          </View>
        </ScrollView>
      </SafeAreaView>
    );
  }

  if (showSelection) {
    return (
      <SafeAreaView style={styles.container}>
        <ScrollView contentContainerStyle={styles.scrollContent}>
          <View style={styles.card}>
            <Text style={styles.cardTitle}>Select Codespace</Text>
            <Text style={styles.cardDescription}>
              Multiple codespaces found. Please select one:
            </Text>
            {codespaces.map((cs, index) => (
              <TouchableOpacity
                key={index}
                style={styles.codespaceItem}
                onPress={() => {
                  resetState();
                  connectToCodespace(orgName, cs.name);
                }}
              >
                <Text style={styles.codespaceTitle}>
                  {cs.name.replace(/-/g, ' ')}
                </Text>
                {cs.repository && (
                  <Text style={styles.codespaceRepo}>{cs.repository}</Text>
                )}
                <Text style={styles.codespaceDate}>
                  Last used: {new Date(cs.lastUsed).toLocaleDateString()}
                </Text>
              </TouchableOpacity>
            ))}
            <TouchableOpacity style={styles.button} onPress={resetState}>
              <Text style={styles.buttonText}>Back</Text>
            </TouchableOpacity>
          </View>
        </ScrollView>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={styles.scrollContent}>
        <View style={styles.card}>
          <View style={styles.logoContainer}>
            <Text style={styles.logoText}>🐱</Text>
          </View>
          <Text style={styles.title}>Catnip</Text>
          <Text style={styles.subtitle}>
            {orgName ? `Access GitHub Codespaces in ${orgName}` : 'Access your GitHub Codespaces'}
          </Text>

          <TouchableOpacity
            style={[styles.button, isConnecting && styles.buttonDisabled]}
            onPress={() => connectToCodespace(orgName || undefined)}
            disabled={isConnecting}
          >
            {isConnecting ? (
              <View style={styles.buttonContent}>
                <ActivityIndicator color="#fff" size="small" />
                <Text style={styles.buttonText}>Connecting...</Text>
              </View>
            ) : (
              <Text style={styles.buttonText}>Access My Codespace</Text>
            )}
          </TouchableOpacity>

          {statusMessage && (
            <View style={styles.statusBox}>
              <Text style={styles.statusText}>{statusMessage}</Text>
            </View>
          )}

          {error && (
            <View style={styles.errorBox}>
              <Text style={styles.errorText}>{error}</Text>
            </View>
          )}

          <View style={styles.divider} />

          <Text style={styles.orText}>Or access codespaces in a specific organization:</Text>

          <View style={styles.inputContainer}>
            <TextInput
              style={styles.input}
              placeholder="Organization name (e.g., wandb)"
              placeholderTextColor="#666"
              value={orgName}
              onChangeText={setOrgName}
              editable={!isConnecting}
            />
            <TouchableOpacity
              style={[styles.goButton, (!orgName || isConnecting) && styles.buttonDisabled]}
              onPress={() => connectToCodespace(orgName)}
              disabled={!orgName || isConnecting}
            >
              <Text style={styles.goButtonText}>Go</Text>
            </TouchableOpacity>
          </View>

          <TouchableOpacity style={styles.logoutButton} onPress={handleLogout}>
            <Text style={styles.logoutText}>Logout</Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#0a0a0a',
  },
  scrollContent: {
    flexGrow: 1,
    justifyContent: 'center',
    padding: 24,
  },
  card: {
    backgroundColor: '#1a1a1a',
    borderRadius: 12,
    padding: 24,
    borderWidth: 1,
    borderColor: '#333',
  },
  logoContainer: {
    alignItems: 'center',
    marginBottom: 16,
  },
  logoText: {
    fontSize: 48,
  },
  title: {
    fontSize: 24,
    fontWeight: 'bold',
    color: '#fff',
    textAlign: 'center',
    marginBottom: 8,
  },
  subtitle: {
    fontSize: 14,
    color: '#999',
    textAlign: 'center',
    marginBottom: 24,
  },
  button: {
    backgroundColor: '#7c3aed',
    paddingVertical: 14,
    borderRadius: 8,
    alignItems: 'center',
    marginBottom: 16,
  },
  buttonDisabled: {
    opacity: 0.5,
  },
  buttonContent: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  buttonText: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600',
  },
  statusBox: {
    backgroundColor: 'rgba(59, 130, 246, 0.1)',
    borderWidth: 1,
    borderColor: 'rgba(59, 130, 246, 0.3)',
    borderRadius: 8,
    padding: 12,
    marginBottom: 16,
  },
  statusText: {
    color: '#93bbfc',
    fontSize: 14,
    textAlign: 'center',
  },
  errorBox: {
    backgroundColor: 'rgba(239, 68, 68, 0.1)',
    borderWidth: 1,
    borderColor: 'rgba(239, 68, 68, 0.3)',
    borderRadius: 8,
    padding: 12,
    marginBottom: 16,
  },
  errorText: {
    color: '#fca5a5',
    fontSize: 14,
    textAlign: 'center',
  },
  divider: {
    height: 1,
    backgroundColor: '#333',
    marginVertical: 24,
  },
  orText: {
    color: '#999',
    fontSize: 14,
    marginBottom: 16,
  },
  inputContainer: {
    flexDirection: 'row',
    gap: 8,
    marginBottom: 24,
  },
  input: {
    flex: 1,
    backgroundColor: '#0a0a0a',
    borderWidth: 1,
    borderColor: '#333',
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 12,
    color: '#fff',
    fontSize: 14,
  },
  goButton: {
    backgroundColor: '#4b5563',
    paddingHorizontal: 24,
    borderRadius: 8,
    justifyContent: 'center',
  },
  goButtonText: {
    color: '#fff',
    fontSize: 14,
    fontWeight: '600',
  },
  logoutButton: {
    alignItems: 'center',
  },
  logoutText: {
    color: '#ef4444',
    fontSize: 14,
  },
  cardTitle: {
    fontSize: 20,
    fontWeight: 'bold',
    color: '#fff',
    marginBottom: 8,
  },
  cardDescription: {
    fontSize: 14,
    color: '#999',
    marginBottom: 24,
  },
  setupSteps: {
    marginBottom: 24,
  },
  stepText: {
    color: '#ccc',
    fontSize: 14,
    marginBottom: 12,
  },
  codeBlock: {
    backgroundColor: '#0a0a0a',
    borderRadius: 8,
    padding: 12,
    marginVertical: 8,
  },
  codeText: {
    color: '#4ade80',
    fontFamily: 'monospace',
    fontSize: 12,
  },
  codespaceItem: {
    backgroundColor: '#0a0a0a',
    borderRadius: 8,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: '#333',
  },
  codespaceTitle: {
    color: '#fff',
    fontSize: 16,
    fontWeight: '600',
    marginBottom: 4,
  },
  codespaceRepo: {
    color: '#3b82f6',
    fontSize: 14,
    marginBottom: 4,
  },
  codespaceDate: {
    color: '#666',
    fontSize: 12,
  },
});