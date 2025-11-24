package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/vanpelt/catnip/internal/claude/parser"
	"github.com/vanpelt/catnip/internal/claude/paths"
	"github.com/vanpelt/catnip/internal/models"
)

// Flag variables for reflect command
var (
	reflectJSONOutput bool
	reflectFullOutput bool
)

// Constants for rendering
const (
	MaxContextTokens    = 155000
	ContentPreviewLines = 8 // Number of lines to show in preview
	GaugeWidth          = 24
)

// Color scheme (matching TUI components)
const (
	colorPrimary = "6"  // Cyan
	colorSuccess = "2"  // Green
	colorWarning = "3"  // Yellow
	colorError   = "1"  // Red
	colorMuted   = "8"  // Gray
	colorText    = "15" // White
)

// ReflectOutput is the top-level JSON output structure
type ReflectOutput struct {
	SessionFile   string           `json:"session_file"`
	Stats         ReflectDisplay   `json:"stats"`
	LatestPrompt  *LatestContent   `json:"latest_prompt,omitempty"`
	LatestThought *LatestContent   `json:"latest_thought,omitempty"`
	LatestMessage *LatestContent   `json:"latest_message,omitempty"`
	LatestTodos   []TodoOutput     `json:"latest_todos,omitempty"`
	Skills        []string         `json:"skills,omitempty"`
	SubAgents     []SubAgentOutput `json:"sub_agents,omitempty"`
}

// TodoOutput represents a todo item for JSON output
type TodoOutput struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// SubAgentOutput represents sub-agent information for JSON output
type SubAgentOutput struct {
	Type          string         `json:"type"`
	Description   string         `json:"description"`
	SessionID     string         `json:"session_id,omitempty"`
	LatestThought *LatestContent `json:"latest_thought,omitempty"`
	LatestMessage *LatestContent `json:"latest_message,omitempty"`
}

// LatestContent represents the simplified structure for latest_* fields
type LatestContent struct {
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
	Content   string `json:"content"`
}

// ReflectDisplay is a version of SessionStats with durations as seconds
type ReflectDisplay struct {
	TotalMessages         int            `json:"total_messages"`
	UserMessages          int            `json:"user_messages"`
	AssistantMessages     int            `json:"assistant_messages"`
	HumanPromptCount      int            `json:"human_prompt_count"`
	ToolCallCount         int            `json:"tool_call_count"`
	TotalInputTokens      int64          `json:"total_input_tokens"`
	TotalOutputTokens     int64          `json:"total_output_tokens"`
	CacheReadTokens       int64          `json:"cache_read_tokens"`
	CacheCreationTokens   int64          `json:"cache_creation_tokens"`
	LastContextSizeTokens int64          `json:"last_context_size_tokens"`
	APICallCount          int            `json:"api_call_count"`
	SessionDurationSecs   float64        `json:"session_duration_secs"`
	ActiveDurationSecs    float64        `json:"active_duration_secs"`
	ThinkingBlockCount    int            `json:"thinking_block_count"`
	SubAgentCount         int            `json:"sub_agent_count"`
	CompactionCount       int            `json:"compaction_count"`
	ImageCount            int            `json:"image_count"`
	FirstMessageTime      string         `json:"first_message_time"`
	LastMessageTime       string         `json:"last_message_time"`
	ActiveToolNames       map[string]int `json:"active_tool_names"`
}

var reflectCmd = &cobra.Command{
	Use:   "reflect [session-file-or-uuid]",
	Short: "Reflect on a Claude session",
	Long: `# Claude Session Reflection

Reflect on a Claude session with detailed statistics and insights.

When running in a terminal (TTY), displays a rich formatted summary.
Use --json to force JSON output.

## Usage

If no argument is provided, the command will find the most recently modified
session file for the git repository root (checking up to 3 parent directories).

You can specify either:
- A path to a .jsonl session file
- A session UUID (will look in the detected project directory)

## Output Structure

` + "```json" + `
{
  "session_file": "/path/to/session.jsonl",
  "stats": { ... },
  "latest_prompt": { "uuid": "...", "timestamp": "...", "content": "..." },
  "latest_thought": { "uuid": "...", "timestamp": "...", "content": "..." },
  "latest_message": { "uuid": "...", "timestamp": "...", "content": "..." }
}
` + "```" + `

## Examples

Reflect on auto-detected (most recent) session:
` + "```bash\ncatnip reflect\n```" + `

Reflect on a specific session file:
` + "```bash\ncatnip reflect /path/to/session.jsonl\n```" + `

Reflect on a session by UUID:
` + "```bash\ncatnip reflect cf568042-7147-4fba-a2ca-c6a646581260\n```",
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var sessionFile string
		var err error

		if len(args) > 0 {
			sessionFile, err = resolveSessionArg(args[0])
			if err != nil {
				outputReflectError(err)
				os.Exit(1)
			}
		} else {
			sessionFile, err = findLatestSessionFileForCWD()
			if err != nil {
				outputReflectError(err)
				os.Exit(1)
			}
		}

		if shouldUseJSON() {
			if err := showReflectJSON(sessionFile); err != nil {
				outputReflectError(err)
				os.Exit(1)
			}
		} else {
			if err := showReflectPretty(sessionFile); err != nil {
				outputReflectError(err)
				os.Exit(1)
			}
		}
	},
}

// shouldUseJSON returns true if JSON output should be used
func shouldUseJSON() bool {
	if reflectJSONOutput {
		return true
	}
	return !isatty.IsTerminal(os.Stdout.Fd())
}

// outputReflectError outputs an error appropriately based on output mode
func outputReflectError(err error) {
	if shouldUseJSON() {
		// JSON mode: output to stdout as JSON
		errOutput := map[string]string{"error": err.Error()}
		jsonOut, _ := json.MarshalIndent(errOutput, "", "  ")
		fmt.Println(string(jsonOut))
	} else {
		// TTY mode: plain text to stderr
		errorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorError))
		fmt.Fprintln(os.Stderr, errorStyle.Render("Error: ")+err.Error())
	}
}

// resolveSessionArg resolves a session argument to a file path
// It accepts either a path to a .jsonl file or a session UUID
func resolveSessionArg(arg string) (string, error) {
	projectDir, err := getProjectDir()
	if err != nil {
		return "", err
	}
	return paths.ResolveSessionPath(arg, projectDir)
}

// getProjectDir returns the Claude projects directory for the current working directory
func getProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Try to find git root (up to 3 levels up)
	projectRoot := paths.FindGitRoot(cwd, 3)
	if projectRoot == "" {
		projectRoot = cwd
	}

	return paths.GetProjectDir(projectRoot)
}

func showReflectJSON(sessionFile string) error {
	// Create reader and read full file
	reader := parser.NewSessionFileReader(sessionFile)
	if err := reader.ReadFull(); err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	// Get stats
	stats := reader.GetStats()

	// Build output structure
	output := ReflectOutput{
		SessionFile: sessionFile,
		Stats: ReflectDisplay{
			TotalMessages:         stats.TotalMessages,
			UserMessages:          stats.UserMessages,
			AssistantMessages:     stats.AssistantMessages,
			HumanPromptCount:      stats.HumanPromptCount,
			ToolCallCount:         stats.ToolCallCount,
			TotalInputTokens:      stats.TotalInputTokens,
			TotalOutputTokens:     stats.TotalOutputTokens,
			CacheReadTokens:       stats.CacheReadTokens,
			CacheCreationTokens:   stats.CacheCreationTokens,
			LastContextSizeTokens: stats.LastContextSizeTokens,
			APICallCount:          stats.APICallCount,
			SessionDurationSecs:   stats.SessionDuration.Seconds(),
			ActiveDurationSecs:    stats.ActiveDuration.Seconds(),
			ThinkingBlockCount:    stats.ThinkingBlockCount,
			SubAgentCount:         stats.SubAgentCount,
			CompactionCount:       stats.CompactionCount,
			ImageCount:            stats.ImageCount,
			FirstMessageTime:      stats.FirstMessageTime.Format("2006-01-02T15:04:05Z07:00"),
			LastMessageTime:       stats.LastMessageTime.Format("2006-01-02T15:04:05Z07:00"),
			ActiveToolNames:       stats.ActiveToolNames,
		},
	}

	// Get latest user prompt from the session file
	latestPrompt := getLatestUserPrompt(sessionFile)
	if latestPrompt != nil {
		output.LatestPrompt = latestPrompt
	}

	// Get latest thought
	latestThought := reader.GetLatestThought()
	if latestThought != nil {
		thinkingBlocks := parser.ExtractThinking(*latestThought)
		if len(thinkingBlocks) > 0 {
			thinking := thinkingBlocks[len(thinkingBlocks)-1].Content
			output.LatestThought = &LatestContent{
				UUID:      latestThought.Uuid,
				Timestamp: latestThought.Timestamp,
				Content:   thinking,
			}
		}
	}

	// Get latest message
	latestMsg := reader.GetLatestMessage()
	if latestMsg != nil {
		textContent := parser.ExtractTextContent(*latestMsg)
		output.LatestMessage = &LatestContent{
			UUID:      latestMsg.Uuid,
			Timestamp: latestMsg.Timestamp,
			Content:   textContent,
		}
	}

	// Get latest todos
	todos := reader.GetTodos()
	if len(todos) > 0 {
		output.LatestTodos = make([]TodoOutput, len(todos))
		for i, todo := range todos {
			output.LatestTodos[i] = TodoOutput{
				Content: todo.Content,
				Status:  todo.Status,
			}
		}
	}

	// Get skills
	skillNames := extractSkillNames(sessionFile)
	if len(skillNames) > 0 {
		output.Skills = skillNames
	}

	// Get sub-agents with peek into their sessions
	subAgents := reader.GetSubAgents()
	if len(subAgents) > 0 {
		sessionDir := filepath.Dir(sessionFile)
		output.SubAgents = make([]SubAgentOutput, 0, len(subAgents))

		for _, agent := range subAgents {
			subAgentOut := SubAgentOutput{
				Type:        agent.SubagentType,
				Description: agent.Description,
				SessionID:   agent.SessionID,
			}

			// Try to peek into sub-agent's session for latest thought/message
			if agent.SessionID != "" && paths.IsValidSessionUUID(agent.SessionID) {
				subAgentFile := filepath.Join(sessionDir, agent.SessionID+".jsonl")
				if _, err := os.Stat(subAgentFile); err == nil {
					subReader := parser.NewSessionFileReader(subAgentFile)
					if err := subReader.ReadFull(); err == nil {
						// Get sub-agent's latest thought
						if subThought := subReader.GetLatestThought(); subThought != nil {
							thinkingBlocks := parser.ExtractThinking(*subThought)
							if len(thinkingBlocks) > 0 {
								thinking := thinkingBlocks[len(thinkingBlocks)-1].Content
								subAgentOut.LatestThought = &LatestContent{
									UUID:      subThought.Uuid,
									Timestamp: subThought.Timestamp,
									Content:   thinking,
								}
							}
						}

						// Get sub-agent's latest message
						if subMsg := subReader.GetLatestMessage(); subMsg != nil {
							textContent := parser.ExtractTextContent(*subMsg)
							if textContent != "" {
								subAgentOut.LatestMessage = &LatestContent{
									UUID:      subMsg.Uuid,
									Timestamp: subMsg.Timestamp,
									Content:   textContent,
								}
							}
						}
					}
				}
			}

			output.SubAgents = append(output.SubAgents, subAgentOut)
		}
	}

	// Output as JSON
	jsonOut, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(jsonOut))
	return nil
}

// getLatestUserPrompt reads the session file and extracts the latest user message with text content
// (skipping tool results which have array content)
func getLatestUserPrompt(sessionFile string) *LatestContent {
	reader := parser.NewSessionFileReader(sessionFile)

	// Use a filter that only gets user messages
	filter := parser.MessageFilter{
		SkipWarmup:    true,
		SkipAutomated: true,
		SkipSidechain: true,
		OnlyType:      "user",
	}

	messages, err := reader.GetAllMessages(filter)
	if err != nil || len(messages) == 0 {
		return nil
	}

	// Iterate backwards to find the last user message with string content
	// (tool results have array content, real prompts have string content)
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Message == nil {
			continue
		}

		c, exists := msg.Message["content"]
		if !exists {
			continue
		}

		contentStr, ok := c.(string)
		if !ok || contentStr == "" {
			continue
		}

		return &LatestContent{
			UUID:      msg.Uuid,
			Timestamp: msg.Timestamp,
			Content:   contentStr,
		}
	}

	return nil
}

// findLatestSessionFileForCWD finds the latest Claude session file for the current working directory
// It checks up to 3 parent directories to find a git repository root
func findLatestSessionFileForCWD() (string, error) {
	projectDir, err := getProjectDir()
	if err != nil {
		return "", err
	}
	return paths.FindBestSessionFile(projectDir)
}

// ============================================================================
// Pretty Output Rendering
// ============================================================================

// Styles for pretty output
var (
	sectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorPrimary))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSuccess))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorWarning))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError))
)

// showReflectPretty displays stats in a rich formatted output
func showReflectPretty(sessionFile string) error {
	// Create reader and read full file
	reader := parser.NewSessionFileReader(sessionFile)
	if err := reader.ReadFull(); err != nil {
		return fmt.Errorf("failed to read session file: %w", err)
	}

	stats := reader.GetStats()

	// Extract skill names
	skillNames := extractSkillNames(sessionFile)

	var sb strings.Builder

	// Session Overview (including context, activity stats)
	sb.WriteString(renderSectionHeader("Session Overview"))
	sb.WriteString(renderSessionTimes(stats.FirstMessageTime, stats.LastMessageTime))
	sb.WriteString("\n")
	sb.WriteString(renderDuration(stats.SessionDuration, stats.ActiveDuration))
	sb.WriteString("\n")
	sb.WriteString(renderContextStatus(stats.LastContextSizeTokens))
	sb.WriteString("\n")
	sb.WriteString(renderOverviewStats(stats.HumanPromptCount, stats.APICallCount, stats.ImageCount))
	sb.WriteString("\n")
	sb.WriteString(renderSecondaryOverview(stats.CompactionCount, stats.ThinkingBlockCount, stats.SubAgentCount))
	sb.WriteString("\n\n")

	// Latest Prompt (right after overview)
	latestPrompt := getLatestUserPrompt(sessionFile)
	if latestPrompt != nil && latestPrompt.Content != "" {
		sb.WriteString(renderSectionHeader("Latest Prompt"))
		sb.WriteString(renderLatestContent(latestPrompt.Content))
		sb.WriteString("\n")
	}

	// Latest Thought
	latestThought := reader.GetLatestThought()
	if latestThought != nil {
		thinkingBlocks := parser.ExtractThinking(*latestThought)
		if len(thinkingBlocks) > 0 {
			thinking := thinkingBlocks[len(thinkingBlocks)-1].Content
			sb.WriteString(renderSectionHeader("Latest Thought"))
			sb.WriteString(renderLatestContent(thinking))
			sb.WriteString("\n")
		}
	}

	// Latest Message
	latestMsg := reader.GetLatestMessage()
	if latestMsg != nil {
		textContent := parser.ExtractTextContent(*latestMsg)
		if textContent != "" {
			sb.WriteString(renderSectionHeader("Latest Message"))
			sb.WriteString(renderLatestContent(textContent))
			sb.WriteString("\n")
		}
	}

	// Latest Todos
	todos := reader.GetTodos()
	if len(todos) > 0 {
		sb.WriteString(renderSectionHeader("Latest Todos"))
		sb.WriteString(renderTodos(todos))
		sb.WriteString("\n")
	}

	// Skills (if any used)
	if len(skillNames) > 0 {
		sb.WriteString(renderSectionHeader("Skills"))
		sb.WriteString(renderSkillsList(skillNames))
		sb.WriteString("\n")
	}

	// Tools (main session)
	if len(stats.ActiveToolNames) > 0 {
		sb.WriteString(renderSectionHeader("Tools"))
		sb.WriteString(renderToolsChart(stats.ActiveToolNames))
	}

	// Sub-agents with peek into their sessions
	subAgents := reader.GetSubAgents()
	if len(subAgents) > 0 {
		sb.WriteString("\n")
		sb.WriteString(renderSectionHeader("Sub-agents"))
		sb.WriteString(renderSubAgentsList(sessionFile, subAgents))
	}

	fmt.Print(sb.String())
	return nil
}

// extractSkillNames extracts skill names from the session by parsing Skill tool calls
func extractSkillNames(sessionFile string) []string {
	reader := parser.NewSessionFileReader(sessionFile)

	filter := parser.MessageFilter{
		SkipWarmup:    true,
		SkipAutomated: false,
		SkipSidechain: false,
	}

	messages, err := reader.GetAllMessages(filter)
	if err != nil {
		return nil
	}

	skillSet := make(map[string]bool)
	for _, msg := range messages {
		toolCalls := parser.ExtractToolCalls(msg)
		for _, toolCall := range toolCalls {
			if toolCall.Name == "Skill" {
				if skillData, exists := toolCall.Input["skill"]; exists {
					if skillName, ok := skillData.(string); ok && skillName != "" {
						skillSet[skillName] = true
					}
				}
			}
		}
	}

	var skills []string
	for skill := range skillSet {
		skills = append(skills, skill)
	}
	sort.Strings(skills)
	return skills
}

// renderSectionHeader renders a section header
func renderSectionHeader(title string) string {
	header := sectionHeaderStyle.Render("═══ " + title + " ═══")
	return header + "\n"
}

// renderSessionTimes renders the start and end times
func renderSessionTimes(start, end time.Time) string {
	startStr := start.Format("Jan 2, 2006 3:04 PM")
	endStr := end.Format("3:04 PM")

	// If different days, show full date for end too
	if !start.Truncate(24 * time.Hour).Equal(end.Truncate(24 * time.Hour)) {
		endStr = end.Format("Jan 2, 2006 3:04 PM")
	}

	return labelStyle.Render("Started: ") + valueStyle.Render(startStr) +
		labelStyle.Render(" │ ") +
		labelStyle.Render("Ended: ") + valueStyle.Render(endStr)
}

// renderDuration renders the duration info as simple text
func renderDuration(session, active time.Duration) string {
	if session == 0 {
		return labelStyle.Render("Duration: ") + valueStyle.Render("N/A")
	}

	percent := float64(active) / float64(session) * 100
	if percent > 100 {
		percent = 100
	}

	sessionStr := formatDuration(session)
	activeStr := formatDuration(active)
	percentStr := fmt.Sprintf("%.0f%%", percent)

	return labelStyle.Render("Duration: ") +
		valueStyle.Render(sessionStr) + labelStyle.Render(" total, ") +
		valueStyle.Render(activeStr) + labelStyle.Render(" active (") +
		valueStyle.Render(percentStr) + labelStyle.Render(")")
}

// renderContextStatus renders context usage as simple text with status
func renderContextStatus(tokens int64) string {
	percent := float64(tokens) / float64(MaxContextTokens) * 100
	if percent > 100 {
		percent = 100
	}

	// Color and status based on thresholds (same style as compaction badge)
	var statusStyle lipgloss.Style
	var statusCircle string
	switch {
	case percent < 50:
		statusStyle = successStyle
		statusCircle = "🟢"
	case percent < 80:
		statusStyle = warningStyle
		statusCircle = "🟡"
	default:
		statusStyle = errorStyle
		statusCircle = "🔴"
	}

	return labelStyle.Render("Context: ") +
		valueStyle.Render(formatNumber(tokens)) +
		labelStyle.Render(" / ") +
		valueStyle.Render(formatNumber(int64(MaxContextTokens))) +
		labelStyle.Render(" tokens (") +
		valueStyle.Render(fmt.Sprintf("%.0f%%", percent)) +
		labelStyle.Render(") ") +
		statusStyle.Render(statusCircle)
}

// renderOverviewStats renders human prompts, API calls, and images
func renderOverviewStats(humanPrompts, apiCalls, images int) string {
	result := labelStyle.Render("Prompts: ") +
		valueStyle.Render(fmt.Sprintf("%d", humanPrompts)) +
		labelStyle.Render(" │ API calls: ") +
		valueStyle.Render(fmt.Sprintf("%d", apiCalls))

	if images > 0 {
		result += labelStyle.Render(" │ Images: ") +
			valueStyle.Render(fmt.Sprintf("%d", images))
	}

	return result
}

// renderSecondaryOverview renders compaction, thinking blocks, and sub-agents
func renderSecondaryOverview(compaction, thinking, subAgents int) string {
	return labelStyle.Render("Compaction: ") + renderCompactionBadge(compaction) +
		labelStyle.Render(" │ Thinking blocks: ") + valueStyle.Render(fmt.Sprintf("%d", thinking)) +
		labelStyle.Render(" │ Sub-agents: ") + valueStyle.Render(fmt.Sprintf("%d", subAgents))
}

// renderTodos renders the current todo list
func renderTodos(todos []models.Todo) string {
	var sb strings.Builder
	for _, todo := range todos {
		var statusIcon string
		var statusStyle lipgloss.Style
		switch todo.Status {
		case "completed":
			statusIcon = "✓"
			statusStyle = successStyle
		case "in_progress":
			statusIcon = "●"
			statusStyle = warningStyle
		default: // pending
			statusIcon = "○"
			statusStyle = labelStyle
		}
		sb.WriteString("  " + statusStyle.Render(statusIcon) + " " + valueStyle.Render(todo.Content) + "\n")
	}
	return sb.String()
}

// renderSkillsList renders the list of skills used
func renderSkillsList(skills []string) string {
	var sb strings.Builder
	for _, skill := range skills {
		sb.WriteString("  " + successStyle.Render("•") + " " + valueStyle.Render(skill) + "\n")
	}
	return sb.String()
}

// renderSubAgentsList renders sub-agents with their type, description, and peek into their session
func renderSubAgentsList(sessionFile string, subAgents []*parser.SubAgentInfo) string {
	sessionDir := filepath.Dir(sessionFile)
	var sb strings.Builder

	for i, agent := range subAgents {
		// Add blank line between sub-agents (not before first one)
		if i > 0 {
			sb.WriteString("\n")
		}

		// Format: Type(description)
		agentLabel := agent.SubagentType
		if agent.Description != "" {
			agentLabel += "(" + agent.Description + ")"
		}
		sb.WriteString("  " + successStyle.Render("▸") + " " + valueStyle.Render(agentLabel) + "\n")

		// Try to peek into sub-agent's session
		if agent.SessionID != "" && paths.IsValidSessionUUID(agent.SessionID) {
			subAgentFile := filepath.Join(sessionDir, agent.SessionID+".jsonl")
			if _, err := os.Stat(subAgentFile); err == nil {
				subReader := parser.NewSessionFileReader(subAgentFile)
				if err := subReader.ReadFull(); err == nil {
					// Show a brief peek into the sub-agent's latest thought
					if subThought := subReader.GetLatestThought(); subThought != nil {
						thinkingBlocks := parser.ExtractThinking(*subThought)
						if len(thinkingBlocks) > 0 {
							thinking := thinkingBlocks[len(thinkingBlocks)-1].Content
							// Truncate to first line or 80 chars
							preview := truncateToLine(thinking, 80)
							sb.WriteString("    " + labelStyle.Render("thought: ") + labelStyle.Render(preview) + "\n")
						}
					}

					// Show a brief peek into the sub-agent's latest message
					if subMsg := subReader.GetLatestMessage(); subMsg != nil {
						textContent := parser.ExtractTextContent(*subMsg)
						if textContent != "" {
							preview := truncateToLine(textContent, 80)
							sb.WriteString("    " + labelStyle.Render("message: ") + labelStyle.Render(preview) + "\n")
						}
					}
				}
			}
		}
	}

	return sb.String()
}

// truncateToLine truncates text to the first line or maxLen characters, whichever is shorter
func truncateToLine(text string, maxLen int) string {
	// Find first newline
	if idx := strings.Index(text, "\n"); idx != -1 && idx < maxLen {
		text = text[:idx]
	}
	if len(text) > maxLen {
		text = text[:maxLen-3] + "..."
	}
	return text
}

// renderCompactionBadge renders the compaction count with color indicator
func renderCompactionBadge(count int) string {
	countStr := fmt.Sprintf("%d", count)
	switch {
	case count == 0:
		return successStyle.Render(countStr + " 🟢")
	case count <= 2:
		return warningStyle.Render(countStr + " 🟡")
	default:
		return errorStyle.Render(countStr + " 🔴")
	}
}

// renderToolsChart renders a bar chart of tool usage
func renderToolsChart(tools map[string]int) string {
	if len(tools) == 0 {
		return ""
	}

	// Sort tools by count descending
	type toolCount struct {
		name  string
		count int
	}
	var sorted []toolCount
	maxCount := 0
	for name, count := range tools {
		sorted = append(sorted, toolCount{name, count})
		if count > maxCount {
			maxCount = count
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	// Only show top 8 tools
	if len(sorted) > 8 {
		sorted = sorted[:8]
	}

	// Find the longest tool name for alignment
	maxNameLen := 0
	for _, tc := range sorted {
		if len(tc.name) > maxNameLen {
			maxNameLen = len(tc.name)
		}
	}

	// Render bars
	var sb strings.Builder
	barWidth := 16
	for _, tc := range sorted {
		// Pad name for alignment
		paddedName := fmt.Sprintf("%*s", maxNameLen, tc.name)

		// Calculate bar length
		barLen := 0
		if maxCount > 0 {
			barLen = int(float64(tc.count) / float64(maxCount) * float64(barWidth))
		}
		if barLen < 1 && tc.count > 0 {
			barLen = 1
		}

		bar := successStyle.Render(strings.Repeat("█", barLen)) +
			labelStyle.Render(strings.Repeat("░", barWidth-barLen))

		sb.WriteString("  " + labelStyle.Render(paddedName) + "  " + bar + " " +
			valueStyle.Render(fmt.Sprintf("%d", tc.count)) + "\n")
	}

	return sb.String()
}

// renderLatestContent renders latest content with optional truncation and markdown
func renderLatestContent(content string) string {
	displayContent := content

	// Truncate by line count if not full output
	truncated := false
	if !reflectFullOutput {
		lines := strings.Split(content, "\n")
		if len(lines) > ContentPreviewLines {
			displayContent = strings.Join(lines[:ContentPreviewLines], "\n")
			truncated = true
		}
	}

	// Try to render with glamour
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err == nil {
		rendered, err := renderer.Render(displayContent)
		if err == nil {
			displayContent = strings.TrimSpace(rendered)
		}
	}

	result := displayContent
	if truncated {
		result += "\n" + labelStyle.Render("[truncated, use --full for complete content]")
	}

	return result + "\n"
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs > 0 {
			return fmt.Sprintf("%dm %ds", mins, secs)
		}
		return fmt.Sprintf("%dm", mins)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dh", hours)
}

// formatNumber formats a number with commas for readability
func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d,%03d,%03d", n/1000000, (n/1000)%1000, n%1000)
}

func init() {
	reflectCmd.Flags().BoolVar(&reflectJSONOutput, "json", false, "Output as JSON (default when not in TTY)")
	reflectCmd.Flags().BoolVar(&reflectFullOutput, "full", false, "Show full content instead of truncated previews")
	rootCmd.AddCommand(reflectCmd)
}
