package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/vanpelt/catnip/internal/logger"
)

// InferenceState represents the current state of the inference service
type InferenceState string

// Inference service states
const (
	InferenceStateInitializing InferenceState = "initializing"
	InferenceStateReady        InferenceState = "ready"
	InferenceStateFailed       InferenceState = "failed"
	InferenceStateDisabled     InferenceState = "disabled"
)

// DownloadProgress tracks the progress of library and model downloads
type DownloadProgress struct {
	LibraryPercent int    `json:"library"`
	ModelPercent   int    `json:"model"`
	CurrentStep    string `json:"step"` // "library", "model", "loading"
}

// InferenceService handles local GGUF model inference using llama.cpp
type InferenceService struct {
	modelPath   string
	libraryPath string
	model       llama.Model
	mu          sync.Mutex
	initialized bool

	// State management
	state        atomic.Value // InferenceState
	stateMessage string
	stateMu      sync.RWMutex
	progress     DownloadProgress
	progressMu   sync.RWMutex

	// Configuration for async init
	config     InferenceConfig
	maxRetries int
}

// InferenceConfig holds configuration for the inference service
type InferenceConfig struct {
	ModelPath   string
	LibraryPath string
	ModelURL    string
	Checksum    string
}

// NewInferenceService creates a new inference service instance (non-blocking)
func NewInferenceService(config InferenceConfig) *InferenceService {
	svc := &InferenceService{
		modelPath:   config.ModelPath,
		libraryPath: config.LibraryPath,
		config:      config,
		maxRetries:  3,
	}
	svc.state.Store(InferenceStateInitializing)
	svc.setStateMessage("Waiting to start initialization...")
	return svc
}

// InitializeAsync starts the background initialization process
func (s *InferenceService) InitializeAsync() {
	var lastErr error

	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		if attempt > 1 {
			// Exponential backoff: 2s, 4s, 8s
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			s.setStateMessage(fmt.Sprintf("Retry %d/%d in %v...", attempt, s.maxRetries, backoff))
			time.Sleep(backoff)
		}

		err := s.initializeOnce()
		if err == nil {
			s.state.Store(InferenceStateReady)
			s.setStateMessage("Inference service ready")
			logger.Infof("🧠 Inference service initialized successfully")
			return
		}

		lastErr = err
		logger.Warnf("🧠 Inference initialization attempt %d/%d failed: %v", attempt, s.maxRetries, err)
	}

	// All retries exhausted
	s.state.Store(InferenceStateFailed)
	s.setStateMessage(fmt.Sprintf("Failed after %d attempts: %v", s.maxRetries, lastErr))
	logger.Errorf("🧠 Inference service failed to initialize after %d attempts: %v", s.maxRetries, lastErr)
}

// initializeOnce performs a single initialization attempt
func (s *InferenceService) initializeOnce() error {
	// Step 1: Download/detect library
	s.setProgress(0, 0, "library")
	s.setStateMessage("Downloading llama.cpp libraries...")

	if s.libraryPath == "" {
		libPath, err := s.downloadOrDetectLibrary()
		if err != nil {
			return fmt.Errorf("failed to get library: %w", err)
		}
		s.libraryPath = libPath
	}
	s.setProgress(100, 0, "model")

	// Step 2: Download model
	s.setStateMessage("Downloading model...")

	if s.config.ModelURL != "" && s.modelPath == "" {
		downloader, err := NewModelDownloader()
		if err != nil {
			return fmt.Errorf("failed to create downloader: %w", err)
		}

		modelFilename := "gemma3-270m-summarizer-Q4_K_M.gguf"
		modelPath, err := downloader.DownloadModel(s.config.ModelURL, modelFilename, s.config.Checksum)
		if err != nil {
			return fmt.Errorf("failed to download model: %w", err)
		}
		s.modelPath = modelPath
	}
	s.setProgress(100, 100, "loading")

	// Step 3: Load model
	s.setStateMessage("Loading model...")

	if err := s.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize model: %w", err)
	}

	return nil
}

// downloadOrDetectLibrary attempts to find or download the llama.cpp library
func (s *InferenceService) downloadOrDetectLibrary() (string, error) {
	// Check environment variable first
	if libPath := os.Getenv("YZMA_LIB"); libPath != "" {
		if _, err := os.Stat(libPath); err == nil {
			return libPath, nil
		}
	}

	// Try auto-download to ~/.catnip/lib
	downloader, err := NewLibraryDownloader()
	if err != nil {
		return "", err
	}

	// Check if library already exists
	libPath, err := downloader.GetLibraryPath()
	if err != nil {
		return "", err
	}

	if _, statErr := os.Stat(libPath); statErr == nil {
		// Library already downloaded
		return libPath, nil
	}

	// Download it
	logger.Infof("🔍 Downloading llama.cpp library...")
	libPath, err = downloader.DownloadLibrary()
	if err != nil {
		return "", err
	}

	return libPath, nil
}

// setStateMessage updates the state message thread-safely
func (s *InferenceService) setStateMessage(msg string) {
	s.stateMu.Lock()
	s.stateMessage = msg
	s.stateMu.Unlock()
}

// setProgress updates the download progress thread-safely
func (s *InferenceService) setProgress(library, model int, step string) {
	s.progressMu.Lock()
	s.progress = DownloadProgress{
		LibraryPercent: library,
		ModelPercent:   model,
		CurrentStep:    step,
	}
	s.progressMu.Unlock()
}

// GetState returns the current state of the service
func (s *InferenceService) GetState() InferenceState {
	return s.state.Load().(InferenceState)
}

// GetStatus returns the current status for the API
func (s *InferenceService) GetStatus() (InferenceState, string, DownloadProgress) {
	state := s.GetState()

	s.stateMu.RLock()
	message := s.stateMessage
	s.stateMu.RUnlock()

	s.progressMu.RLock()
	progress := s.progress
	s.progressMu.RUnlock()

	return state, message, progress
}

// IsReady returns true if the service is ready for inference
func (s *InferenceService) IsReady() bool {
	return s.GetState() == InferenceStateReady
}

// Initialize loads the library and model
func (s *InferenceService) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	// Suppress llama.cpp's verbose stderr output
	suppressStderr()

	// Extract directory from library path
	// yzma.Load() expects the directory containing the libraries, not a specific file
	libDir := filepath.Dir(s.libraryPath)

	// Load llama.cpp library (pass directory, not file)
	if err := llama.Load(libDir); err != nil {
		restoreStderr()
		return fmt.Errorf("failed to load llama.cpp library: %w", err)
	}

	// Initialize llama
	llama.Init()

	// Load model
	modelParams := llama.ModelDefaultParams()
	model := llama.ModelLoadFromFile(s.modelPath, modelParams)
	// Note: yzma returns zero-value model on failure, we can't check for nil

	// Restore stderr after initialization
	restoreStderr()

	s.model = model
	s.initialized = true

	return nil
}

// SummarizeResponse contains the summary and suggested branch name
type SummarizeResponse struct {
	Summary    string
	BranchName string
}

// Summarize generates a summary and branch name from the given prompt
func (s *InferenceService) Summarize(prompt string) (*SummarizeResponse, error) {
	// Ensure initialized
	if !s.initialized {
		if err := s.Initialize(); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Suppress llama.cpp's verbose output during inference
	suppressStderr()
	defer restoreStderr()

	// Create context with proper parameters
	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = 32768 // Match model's training context size (as per Unsloth guidance)
	ctx := llama.InitFromModel(s.model, ctxParams)
	defer llama.Free(ctx) // Clean up context when done

	// Get vocabulary
	vocab := llama.ModelGetVocab(s.model)

	// Manually construct prompt EXACTLY like Ollama's template does
	// This bypasses llama.cpp's chat template to match Ollama's behavior precisely
	systemPrompt := `You are a careful assistant that generates task summaries and git branch names.

Output EXACTLY two lines with no other text:
Line 1: A 2-4 word summary (Title Case, no punctuation)
Line 2: A git branch name (kebab-case, lowercase, [a-z0-9-] only, max 4 words, prefix with a category like bug/, feat/, etc.)

Examples:
Fix Login Bug
bug/fix-login

Add Dark Mode
feat/add-dark-mode

API Docs
docs/api-polish

Refactor User Service V2
chore/user-service-v2-refactor

Turn this request for code changes into:
1) a 2-4 word summary (Title Case),
2) a friendly git branch name (prefixed kebab-case).`

	// Construct prompt manually matching Ollama's template exactly:
	// <start_of_turn>user\n{{ $.System }}\n{{ .Content }}<end_of_turn>\n<start_of_turn>model\n
	fullPrompt := "<start_of_turn>user\n" + systemPrompt + "\n" + prompt + "<end_of_turn>\n<start_of_turn>model\n"

	// Tokenize the formatted prompt
	// CRITICAL FIX: Must add special tokens (BOS) for Gemma to work correctly
	addSpecial := true
	parseSpecial := true
	tokens := llama.Tokenize(vocab, fullPrompt, addSpecial, parseSpecial)

	// Create batch
	batch := llama.BatchGetOne(tokens)

	// Setup sampler chain with parameters from Modelfile
	samplerParams := llama.SamplerChainDefaultParams()
	sampler := llama.SamplerChainInit(samplerParams)
	defer llama.SamplerFree(sampler) // Clean up sampler when done

	// Add samplers matching llama.cpp's common_sampler_init order
	// Correct order: TOP_K → TOP_P → TYPICAL_P → TEMPERATURE → PENALTIES → Dist
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(64))                     // top_k=64 (from Modelfile)
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(0.95, 1))                // top_p=0.95 (from Modelfile)
	llama.SamplerChainAdd(sampler, llama.SamplerInitTypical(1.0, 1))              // typical_p=1.0 (Ollama default, min_keep=1)
	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(0.8, 0.0, 1.0))       // temp=0.8 (Ollama default)
	llama.SamplerChainAdd(sampler, llama.SamplerInitPenalties(64, 1.1, 0.0, 0.0)) // repeat_penalty=1.1, repeat_last_n=64

	// Use random seed for variability (Ollama generates new seed per request)
	seed := uint32(time.Now().UnixMicro() & 0xFFFFFFFF) //nolint:gosec // Safe: intentional truncation for seed
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(seed))

	// Generate tokens
	maxTokens := int32(128) // Limit generation
	var output strings.Builder
	buf := make([]byte, 36) // Buffer for token text
	newlineCount := 0

	for pos := int32(0); pos < maxTokens; pos++ {
		// Decode batch
		llama.Decode(ctx, batch)

		// Sample next token
		token := llama.SamplerSample(sampler, ctx, -1)

		// Check for end of generation (EOS token)
		if llama.VocabIsEOG(vocab, token) {
			break
		}

		// Convert token to text
		tokenLen := llama.TokenToPiece(vocab, token, buf, 0, true)
		if tokenLen > 0 {
			output.Write(buf[:tokenLen])
		}

		// Check for stop sequences
		currentOutput := output.String()

		// Stop at <end_of_turn> (from Modelfile)
		if strings.Contains(currentOutput, "<end_of_turn>") {
			// Remove the stop sequence from output
			currentOutput = strings.Split(currentOutput, "<end_of_turn>")[0]
			output.Reset()
			output.WriteString(currentOutput)
			break
		}

		// Count newlines - stop after we have 2 complete lines
		// (We want exactly: Line1\nLine2\n)
		if tokenLen > 0 && buf[0] == '\n' {
			newlineCount++
			// Stop after 2 newlines (which gives us 2 lines of content)
			if newlineCount >= 2 {
				break
			}
		}

		// Create next batch with single token
		batch = llama.BatchGetOne([]llama.Token{token})
	}

	// Get raw output
	rawOutput := output.String()

	// Parse output into summary and branch name
	return s.parseOutput(rawOutput)
}

// parseOutput parses the model output into summary and branch name
func (s *InferenceService) parseOutput(output string) (*SummarizeResponse, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Find first two non-empty lines
	var summary, branchName string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if summary == "" {
			summary = line
		} else if branchName == "" {
			branchName = line
			break
		}
	}

	if summary == "" || branchName == "" {
		return nil, fmt.Errorf("invalid output format: expected 2 lines, got: %s", output)
	}

	return &SummarizeResponse{
		Summary:    summary,
		BranchName: branchName,
	}, nil
}

// Close frees resources
func (s *InferenceService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Note: yzma doesn't expose model cleanup in current API
	s.initialized = false
	return nil
}
