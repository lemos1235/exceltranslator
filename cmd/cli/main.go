package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	"exceltranslator/pkg/config"
	"exceltranslator/pkg/runner"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type status int

const (
	statusConfiguring status = iota
	statusSettings
	statusCommand
	statusProcessing
	statusCancelling // New status for cancellation confirmation
	statusDone
	statusError
)

type model struct {
	inputs     []textinput.Model
	focusedIdx int
	err        error
	appConfig  *config.AppConfig
	progress   progress.Model
	viewport   viewport.Model
	ready      bool
	status     status
	isEditing  bool
	quitting   bool // Flag for double Ctrl+C to quit

	// Command Mode
	cmdInput       textinput.Model
	previousStatus status

	// Suggestions
	suggestions   []string
	suggestionIdx int

	// Translation state
	currentPhase string
	processed    int
	total        int
	logs         []string
	sub          chan translationUpdate
	cancelFunc   context.CancelFunc // Cancel function for translation
}

func expandPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

const (
	inputInputFile = iota
	inputOutputFile
	inputBaseURL
	inputAPIKey
	inputModel
	inputPrompt
	inputCJKOnly
	inputLogLevel
	inputLogDisable

	inputStartTranslation = 100
)

// Messages for translation updates
type translationUpdate interface{}

type progressUpdate struct {
	phase       string
	done, total int
}

type logUpdate struct {
	text string
}

type translatedUpdate struct {
	original   string
	translated string
}

type errorUpdate struct {
	err error
}

type completeUpdate struct{}
type resetQuitMsg struct{}

// Helper to get labels for inputs
func getInputLabel(idx int) string {
	switch idx {
	case inputInputFile:
		return "Input File"
	case inputOutputFile:
		return "Output File"
	case inputBaseURL:
		return "API Base URL"
	case inputAPIKey:
		return "API Key"
	case inputModel:
		return "Model Name"
	case inputPrompt:
		return "System Prompt"
	case inputCJKOnly:
		return "CJK Only (true/false)"
	case inputLogLevel:
		return "Log Level"
	default:
		return "Unknown"
	}
}

// Helper to get description for inputs
func getInputDescription(idx int) string {
	switch idx {
	case inputInputFile:
		return "Enter the path to the Excel file you want to translate."
	case inputOutputFile:
		return "Enter the path where the translated file should be saved."
	case inputBaseURL:
		return "Enter the base URL for the LLM API (e.g., https://api.openai.com/v1)."
	case inputAPIKey:
		return "Enter your LLM API Key."
	case inputModel:
		return "Enter the model name (e.g., gpt-3.5-turbo, qwen-flash)."
	case inputPrompt:
		return "Enter the system prompt to guide the translation."
	case inputCJKOnly:
		return "If true, only translate cells containing CJK characters."
	case inputLogLevel:
		return "Enter the log level (debug, info, warn, error)."
	default:
		return ""
	}
}

func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	p := strs[0]
	for _, s := range strs {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

func (m *model) handleCompletion(idx int) {
	val := m.inputs[idx].Value()

	// Cycle if already suggesting and value matches current suggestion
	if len(m.suggestions) > 1 && m.suggestionIdx >= 0 && m.suggestionIdx < len(m.suggestions) {
		if val == m.suggestions[m.suggestionIdx] {
			m.suggestionIdx = (m.suggestionIdx + 1) % len(m.suggestions)
			m.inputs[idx].SetValue(m.suggestions[m.suggestionIdx])
			m.inputs[idx].CursorEnd()
			return
		}
	}

	// Calculate matches
	var matches []string

	if idx == inputInputFile && strings.HasPrefix(val, ":") {
		// Command mode (only for input file)
		commands := []string{":settings", ":quit"}
		for _, c := range commands {
			if strings.HasPrefix(c, val) {
				matches = append(matches, c)
			}
		}
	} else {
		// File mode
		dir := filepath.Dir(val)
		base := filepath.Base(val)

		// Adjust for root or current dir
		if val == "" {
			dir = "."
			base = ""
		} else if strings.HasSuffix(val, string(os.PathSeparator)) {
			dir = val
			base = ""
		} else if dir == "." && !strings.Contains(val, string(os.PathSeparator)) {
			// val="foo" -> dir=".", base="foo"
		}

		searchDir := dir
		if searchDir == "" {
			searchDir = "."
		}
		searchDir = expandPath(searchDir)

		entries, err := os.ReadDir(searchDir)
		if err == nil {
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), base) {
					// Skip hidden unless typed
					if strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(base, ".") {
						continue
					}

					name := e.Name()
					if e.IsDir() {
						name += string(os.PathSeparator)
					}

					var full string
					if dir == "." {
						full = name
					} else {
						full = filepath.Join(dir, name)
						if e.IsDir() && !strings.HasSuffix(full, string(os.PathSeparator)) {
							full += string(os.PathSeparator)
						}
					}
					matches = append(matches, full)
				}
			}
		}
	}

	sort.Strings(matches)
	m.suggestions = matches
	m.suggestionIdx = -1

	if len(m.suggestions) == 0 {
		return
	}

	if len(m.suggestions) == 1 {
		m.inputs[idx].SetValue(m.suggestions[0])
		m.inputs[idx].CursorEnd()
		m.suggestions = nil
	} else {
		cp := commonPrefix(m.suggestions)
		if len(cp) > len(val) {
			m.inputs[idx].SetValue(cp)
			m.inputs[idx].CursorEnd()
		} else if val == cp {
			// Start cycling
			m.suggestionIdx = 0
			m.inputs[idx].SetValue(m.suggestions[0])
			m.inputs[idx].CursorEnd()
		}
	}
}

func (m *model) nextField() {
	switch m.status {
	case statusConfiguring:
		switch m.focusedIdx {
		case inputOutputFile:
			m.focusedIdx = inputStartTranslation
		case inputStartTranslation:
			m.focusedIdx = inputInputFile
		default:
			m.focusedIdx++
		}
	case statusSettings:
		m.focusedIdx++
		if m.focusedIdx < inputBaseURL || m.focusedIdx > inputCJKOnly {
			m.focusedIdx = inputBaseURL
		}
	default:
		m.focusedIdx++
		if m.focusedIdx >= len(m.inputs) {
			m.focusedIdx = 0
		}
	}
}

func (m *model) prevField() {
	switch m.status {
	case statusConfiguring:
		switch m.focusedIdx {
		case inputStartTranslation:
			m.focusedIdx = inputOutputFile
		case inputInputFile:
			m.focusedIdx = inputStartTranslation
		default:
			m.focusedIdx--
		}
	case statusSettings:
		m.focusedIdx--
		if m.focusedIdx < inputBaseURL || m.focusedIdx > inputCJKOnly {
			m.focusedIdx = inputCJKOnly
		}
	default:
		m.focusedIdx--
		if m.focusedIdx < 0 {
			m.focusedIdx = len(m.inputs) - 1
		}
	}
}

func (m *model) updateFocus() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := 0; i <= len(m.inputs)-1; i++ {
		// Styling dependent on status
		if m.status == statusConfiguring && (i == inputInputFile || i == inputOutputFile) {
			// Main View Styling is handled in View(), but focus state needs to be set
			if i == m.focusedIdx {
				cmds[i] = m.inputs[i].Focus()
			} else {
				m.inputs[i].Blur()
			}
			m.inputs[i].TextStyle = noStyle
			m.inputs[i].Prompt = ""
		} else {
			// Settings View or Default
			if i == m.focusedIdx && m.isEditing {
				cmds[i] = m.inputs[i].Focus()
				m.inputs[i].TextStyle = focusedTextStyle
				m.inputs[i].Prompt = " > "
			} else {
				m.inputs[i].Blur()
				if i == m.focusedIdx {
					m.inputs[i].TextStyle = focusedTextStyle
					m.inputs[i].Prompt = " * "
				} else {
					m.inputs[i].TextStyle = noStyle
					m.inputs[i].Prompt = "   "
				}
			}
		}
	}

	if !m.isEditing {
		m.inputs[inputInputFile].Placeholder = "path/to/source.xlsx"
		m.inputs[inputOutputFile].Placeholder = "path/to/destination.xlsx"
	} else {
		m.inputs[inputInputFile].Placeholder = "path/to/source.xlsx"
		m.inputs[inputOutputFile].Placeholder = "path/to/destination.xlsx"
	}

	return tea.Batch(cmds...)
}

func initialModel() model {
	m := model{
		inputs:        make([]textinput.Model, 8),
		progress:      progress.New(progress.WithDefaultGradient()),
		status:        statusConfiguring,
		suggestionIdx: -1,
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	m.appConfig = cfg

	var t textinput.Model

	// Input File
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 512
	t.Placeholder = "path/to/source.xlsx"
	m.inputs[inputInputFile] = t

	// Output File
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 512
	t.Placeholder = "path/to/destination.xlsx"
	m.inputs[inputOutputFile] = t

	// Base URL
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 256
	t.Placeholder = "https://api.openai.com/v1"
	t.SetValue(cfg.LLM.BaseURL)
	m.inputs[inputBaseURL] = t

	// API Key
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 256
	t.Placeholder = "sk-..."
	t.EchoMode = textinput.EchoPassword
	t.SetValue(cfg.LLM.APIKey)
	m.inputs[inputAPIKey] = t

	// Model
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 256
	t.Placeholder = "gpt-4"
	t.SetValue(cfg.LLM.Model)
	m.inputs[inputModel] = t

	// Prompt
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 1024
	t.Placeholder = "Translate to..."
	t.SetValue(cfg.LLM.Prompt)
	m.inputs[inputPrompt] = t

	// CJK Only
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 5
	t.Placeholder = "false"
	t.SetValue(strconv.FormatBool(cfg.Extractor.CJKOnly))
	m.inputs[inputCJKOnly] = t

	// Log Level
	t = textinput.New()
	t.Cursor.Style = cursorStyle
	t.CharLimit = 10
	t.Placeholder = "info"
	t.SetValue(cfg.Log.Level)
	m.inputs[inputLogLevel] = t

	// Command Input
	ci := textinput.New()
	ci.Prompt = ":"
	ci.Cursor.Style = cursorStyle
	m.cmdInput = ci

	m.isEditing = true
	m.focusedIdx = inputInputFile

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.updateFocus())
}

func (m *model) handleCommandCompletion() {
	val := m.cmdInput.Value()

	// If suggestions exist, cycle through them
	if len(m.suggestions) > 0 {
		m.suggestionIdx = (m.suggestionIdx + 1) % len(m.suggestions)
		m.cmdInput.SetValue(m.suggestions[m.suggestionIdx])
		m.cmdInput.CursorEnd()
		return
	}

	commands := []string{"quit", "settings"}
	var matches []string
	for _, c := range commands {
		if strings.HasPrefix(c, val) {
			matches = append(matches, c)
		}
	}
	sort.Strings(matches)

	if len(matches) == 0 {
		return
	}

	m.suggestions = matches
	m.suggestionIdx = 0
	m.cmdInput.SetValue(matches[0])
	m.cmdInput.CursorEnd()
}

func (m model) executeCommand(cmdStr string) (tea.Model, tea.Cmd) {
	m.status = m.previousStatus // Restore status first, then act

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "q", "quit":
		return m, tea.Quit
	case "settings":
		m.status = statusSettings
		m.focusedIdx = inputBaseURL
		m.isEditing = false
		return m, m.updateFocus()
	}
	return m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// --- Command Mode Handling ---
	if m.status == statusCommand {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEsc:
				m.status = m.previousStatus
				m.cmdInput.SetValue("")
				m.suggestions = nil
				return m, m.updateFocus()
			case tea.KeyEnter:
				m.suggestions = nil
				return m.executeCommand(m.cmdInput.Value())
			case tea.KeyTab:
				m.handleCommandCompletion()
				return m, nil
			}
		}
		oldVal := m.cmdInput.Value()
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		if m.cmdInput.Value() != oldVal {
			m.suggestions = nil
		}
		return m, cmd
	}

	// --- Main Update Loop ---
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if m.quitting {
				return m, tea.Quit
			}
			m.quitting = true
			return m, tea.Tick(time.Second, func(_ time.Time) tea.Msg {
				return resetQuitMsg{}
			})
		}

		// Toggle Command Mode (Global Priority)
		if msg.String() == ":" && m.status != statusCancelling && m.status != statusProcessing {
			m.previousStatus = m.status
			m.status = statusCommand
			m.cmdInput.SetValue("")
			m.cmdInput.Focus()
			return m, textinput.Blink
		}

		// Handle Cancellation Confirmation
		if m.status == statusCancelling {
			switch msg.String() {
			case "y", "Y":
				if m.cancelFunc != nil {
					m.cancelFunc()
					m.cancelFunc = nil
				}
				// m.logs = append(m.logs, "Translation cancelled by user.")
				// m.viewport.SetContent(strings.Join(m.logs, "\n"))
				m.status = statusConfiguring
				return m, m.updateFocus()
			case "n", "N":
				m.status = statusProcessing
				return m, nil
			}
			// Block other keys in this state
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEsc:
			if m.status == statusProcessing {
				m.status = statusCancelling
				return m, nil
			}
			if m.status == statusDone || m.status == statusError {
				m.status = statusConfiguring
				m.focusedIdx = inputInputFile
				m.isEditing = false
				return m, m.updateFocus()
			}
			if m.status == statusConfiguring {
				// In main view, ESC could clear focus or do nothing,
				// but let's not disable isEditing if we want to stay in input mode.
				// For now, let's just make it do nothing or clear the suggestions.
				m.suggestions = nil
				return m, nil
			}
			if m.isEditing {
				m.isEditing = false
				cmd = m.updateFocus()
				cmds = append(cmds, cmd)
				break
			}
			if m.status == statusSettings {
				m.saveConfigFromInputs()
				m.status = statusConfiguring
				m.focusedIdx = inputInputFile
				m.isEditing = false
				cmd = m.updateFocus()
				cmds = append(cmds, cmd)
				break
			}
			return m, nil // Quit on Esc in main view if not editing? Or nothing.

		case tea.KeyUp, tea.KeyDown:
			if m.status == statusConfiguring {
				if msg.Type == tea.KeyUp {
					m.prevField()
				} else {
					m.nextField()
				}
				// Ensure editing mode is on if we moved to an input field
				if m.focusedIdx == inputInputFile || m.focusedIdx == inputOutputFile {
					m.isEditing = true
				} else {
					m.isEditing = false
				}
				cmd = m.updateFocus()
				cmds = append(cmds, cmd)
				break
			}
			if !m.isEditing {
				if msg.Type == tea.KeyUp {
					m.prevField()
				} else {
					m.nextField()
				}
				cmd = m.updateFocus()
				cmds = append(cmds, cmd)
				break
			}

		case tea.KeyTab:
			switch m.status {
			case statusConfiguring:
				if (m.focusedIdx == inputInputFile || m.focusedIdx == inputOutputFile) && m.isEditing {
					// Allow tab to complete file path if editing
					m.handleCompletion(m.focusedIdx)
					return m, nil
				}
				// Disable tab navigation in main interface as requested
				return m, nil
			case statusSettings:
				m.nextField()
				return m, m.updateFocus()
			}

		case tea.KeyEnter:
			// Start Translation Trigger
			if m.focusedIdx == inputStartTranslation {
				// Validation: Check if files are set
				if m.inputs[inputInputFile].Value() == "" || m.inputs[inputOutputFile].Value() == "" {
					return m, nil // Do nothing if files are missing
				}

				m.status = statusProcessing
				m.saveConfigFromInputs()
				m.sub = make(chan translationUpdate)
				m.logs = []string{"Starting translation..."}
				m.viewport.SetContent(strings.Join(m.logs, "\n"))

				ctx, cancel := context.WithCancel(context.Background())
				m.cancelFunc = cancel

				return m, tea.Batch(
					startTranslation(ctx, expandPath(m.inputs[inputInputFile].Value()), expandPath(m.inputs[inputOutputFile].Value()), m.appConfig, m.sub),
					waitForUpdate(m.sub),
				)
			}

			// Edit Mode Toggle (if not already editing and not start button)
			if !m.isEditing {
				m.isEditing = true
				return m, m.updateFocus()
			}

			// Confirming Edit
			if m.status == statusConfiguring {
				// Handle transitions between fields
				if m.focusedIdx == inputInputFile {
					m.isEditing = false // Commit changes first

					// Auto-fill output file
					inputFile := m.inputs[inputInputFile].Value()
					if inputFile != "" && m.inputs[inputOutputFile].Value() == "" {
						ext := filepath.Ext(inputFile)
						base := strings.TrimSuffix(inputFile, ext)
						m.inputs[inputOutputFile].SetValue(base + "_translated" + ext)
					}

					// Move to Output File and auto-enter edit mode
					m.focusedIdx = inputOutputFile
					m.isEditing = true
					cmd = m.updateFocus()
					cmds = append(cmds, cmd)
					break
				} else if m.focusedIdx == inputOutputFile {
					m.isEditing = false // Commit changes

					// Move to Translate Button (no edit mode)
					m.focusedIdx = inputStartTranslation
					cmd = m.updateFocus()
					cmds = append(cmds, cmd)
					break
				}

				m.isEditing = false
				cmd = m.updateFocus()
				cmds = append(cmds, cmd)
			} else if m.status == statusSettings {
				// Handle toggles
				if m.focusedIdx == inputCJKOnly {
					cjk, _ := strconv.ParseBool(m.inputs[inputCJKOnly].Value())
					m.inputs[inputCJKOnly].SetValue(strconv.FormatBool(!cjk))
					break
				}
				m.isEditing = false
				cmd = m.updateFocus()
				cmds = append(cmds, cmd)
				break
			}
		}

	case resetQuitMsg:
		m.quitting = false
		return m, nil

	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

		// Update input widths
		// Calculate a good width for the cards in main view
		cardWidth := max(40, msg.Width-20)

		for i := range m.inputs {
			if i == inputInputFile || i == inputOutputFile {
				m.inputs[i].Width = cardWidth - 6 // Account for padding
			} else {
				m.inputs[i].Width = msg.Width - 10
			}
		}
		m.cmdInput.Width = msg.Width - 10

	case progressUpdate:
		m.currentPhase = msg.phase
		m.processed = msg.done
		m.total = msg.total
		cmd = m.progress.SetPercent(float64(msg.done) / float64(msg.total))
		cmds = append(cmds, cmd)
		cmds = append(cmds, waitForUpdate(m.sub))

	case logUpdate:
		m.logs = append(m.logs, msg.text)
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
		cmds = append(cmds, waitForUpdate(m.sub))

	case translatedUpdate:
		m.logs = append(m.logs, fmt.Sprintf("%s -> %s", msg.original, msg.translated))
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
		cmds = append(cmds, waitForUpdate(m.sub))

	case errorUpdate:
		if errors.Is(msg.err, context.Canceled) {
			m.status = statusConfiguring
			// m.logs = append(m.logs, "Translation cancelled.")
			// m.viewport.SetContent(strings.Join(m.logs, "\n"))
			return m, m.updateFocus()
		}
		m.status = statusError
		m.err = msg.err
		m.logs = append(m.logs, fmt.Sprintf("Error: %v", msg.err))
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		return m, nil

	case completeUpdate:
		m.status = statusDone
		m.logs = append(m.logs, "Translation completed successfully.")
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		return m, nil

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmds = append(cmds, cmd)
	}

	// Update inputs
	var lastVal string
	if m.focusedIdx >= 0 && m.focusedIdx < len(m.inputs) {
		lastVal = m.inputs[m.focusedIdx].Value()
	}
	cmd = m.updateInputs(msg)
	cmds = append(cmds, cmd)

	if m.focusedIdx >= 0 && m.focusedIdx < len(m.inputs) && m.inputs[m.focusedIdx].Value() != lastVal {
		m.suggestions = nil
		m.suggestionIdx = -1
	}

	// Render content based on status
	switch m.status {
	case statusConfiguring:
		m.viewport.SetContent(m.configViewContent())
	case statusSettings:
		m.viewport.SetContent(m.settingsViewContent())
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) saveConfigFromInputs() {
	m.appConfig.LLM.BaseURL = m.inputs[inputBaseURL].Value()
	m.appConfig.LLM.APIKey = m.inputs[inputAPIKey].Value()
	m.appConfig.LLM.Model = m.inputs[inputModel].Value()
	m.appConfig.LLM.Prompt = m.inputs[inputPrompt].Value()

	cjkOnly, _ := strconv.ParseBool(m.inputs[inputCJKOnly].Value())
	m.appConfig.Extractor.CJKOnly = cjkOnly

	_ = config.Save(m.appConfig)
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m model) headerView() string {
	title := titleStyle.Render("Excel Translator")
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (m model) footerView() string {
	if m.quitting {
		return "\n" + helpStyle.Render("Press Ctrl+C again to quit") + "\n"
	}
	if m.status == statusCommand {
		return "\n" + m.cmdInput.View() + "\n"
	}
	if m.status == statusCancelling {
		prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("Cancel translation? (y/n)")
		return "\n" + prompt + "\n"
	}
	if m.status == statusProcessing {
		phase := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(fmt.Sprintf("Phase: %s", m.currentPhase))
		return "\n" + phase + "\n" + m.progress.View() + "\n" + helpStyle.Render("ESC: Cancel") + "\n"
	}
	if m.status == statusDone {
		return "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("Done!") + helpStyle.Render(" Press ESC to return | Ctrl+C: Quit") + "\n"
	}
	if m.status == statusError {
		return "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error!") + helpStyle.Render(" Press ESC to return | Ctrl+C: Quit") + "\n"
	}
	if m.status == statusConfiguring {
		return "\n" + helpStyle.Render(": Command | ↑/↓: Navigate | ENTER: Confirm | Ctrl+C: Quit") + "\n"
	}
	if m.status == statusSettings {
		return "\n" + helpStyle.Render("ESC: Save & Back | ↑/↓: Navigate | ENTER: Toggle/Edit | Ctrl+C: Quit") + "\n"
	}
	return ""
}

// Rewritten Main View
func (m model) configViewContent() string {
	// Styles
	width := max(40, m.viewport.Width-20)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(width)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Bold(true).
		MarginBottom(0)

	// Input Styles
	baseInputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width - 6) // -6 for padding/borders of parent

	activeBorderColor := lipgloss.Color("205")
	inactiveBorderColor := lipgloss.Color("240")

	renderInput := func(idx int, label string) string {
		var s lipgloss.Style
		if m.focusedIdx == idx {
			s = baseInputStyle.Copy().BorderForeground(activeBorderColor)
		} else {
			s = baseInputStyle.Copy().BorderForeground(inactiveBorderColor)
		}

		return lipgloss.JoinVertical(lipgloss.Left,
			labelStyle.Render(label),
			s.Render(m.inputs[idx].View()),
		)
	}

	// Source File
	inFile := renderInput(inputInputFile, "SOURCE FILE")

	// Output File
	outFile := renderInput(inputOutputFile, "DESTINATION FILE")

	// Translate Button
	btnStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Padding(0, 4).
		MarginTop(1)

	var btn string

	filesValid := m.inputs[inputInputFile].Value() != "" && m.inputs[inputOutputFile].Value() != ""

	if m.focusedIdx == inputStartTranslation {
		if filesValid {
			btn = btnStyle.Copy().
				Background(lipgloss.Color("205")).
				Bold(true).
				Render("TRANSLATE")
		} else {
			btn = btnStyle.Copy().
				Background(lipgloss.Color("240")). // Dimmed background even if focused
				Bold(true).
				Foreground(lipgloss.Color("246")). // Dimmed text
				Render("TRANSLATE")
		}
	} else {
		if filesValid {
			btn = btnStyle.Copy().
				Background(lipgloss.Color("240")).
				Render("TRANSLATE")
		} else {
			btn = btnStyle.Copy().
				Background(lipgloss.Color("237")). // Very dim background
				Foreground(lipgloss.Color("243")). // Very dim text
				Render("TRANSLATE")
		}
	}

	// Combine all
	ui := lipgloss.JoinVertical(lipgloss.Center,
		inFile,
		"\n",
		outFile,
		"\n",
		btn,
	)

	return lipgloss.Place(
		m.viewport.Width,
		m.viewport.Height,
		lipgloss.Center,
		lipgloss.Center,
		boxStyle.Render(ui),
	)
}

func (m model) settingsViewContent() string {
	var b strings.Builder

	b.WriteString(groupTitleStyle.Render("LLM Settings") + "\n")
	b.WriteString(m.renderSettingInput(inputBaseURL))
	b.WriteString(m.renderSettingInput(inputAPIKey))
	b.WriteString(m.renderSettingInput(inputModel))
	b.WriteString(m.renderSettingInput(inputPrompt))
	b.WriteString("\n")

	b.WriteString(groupTitleStyle.Render("Extractor Settings") + "\n")
	b.WriteString(m.renderSettingInput(inputCJKOnly))
	b.WriteString("\n")

	return b.String()
}

func (m model) renderSettingInput(idx int) string {
	label := getInputLabel(idx)
	var value string

	if m.focusedIdx == idx && m.isEditing {
		value = m.inputs[idx].View()
	} else {
		val := m.inputs[idx].Value()
		if idx == inputAPIKey && val != "" {
			val = "********"
		}

		if m.focusedIdx == idx {
			value = focusedStyle.Render(val)
			// Add hint for toggleable items
			if idx == inputCJKOnly || idx == inputLogLevel {
				value += helpStyle.Render(" (Enter to toggle)")
			} else {
				value += helpStyle.Render(" (Enter to edit)")
			}
		} else {
			value = val
		}
	}

	// Add a marker if focused
	if m.focusedIdx == idx {
		return fmt.Sprintf("> %s: %s\n", label, value)
	}
	return fmt.Sprintf("  %s: %s\n", label, value)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	// For main view, viewport content is already centered, so we just show it.
	// For settings, it's a list.
	return fmt.Sprintf("%s\n%s%s", m.headerView(), m.viewport.View(), m.footerView())
}

// Styling
var (
	titleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Padding(0, 1)
	descriptionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	groupTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true).Underline(true)
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	focusedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	focusedTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	noStyle          = lipgloss.NewStyle()
	cursorStyle      = focusedStyle
)

// Commands

func waitForUpdate(sub chan translationUpdate) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

func startTranslation(ctx context.Context, input, output string, cfg *config.AppConfig, sub chan translationUpdate) tea.Cmd {
	// Create a copy to avoid modifying the persistent config
	runnerCfg := *cfg
	runnerCfg.Log.Disable = true

	return func() tea.Msg {
		go func() {
			cb := runner.TranslationCallbacks{
				OnTranslated: func(original, translated string) {
					sub <- translatedUpdate{original: original, translated: translated}
				},
				OnProgress: func(phase string, done, total int) {
					sub <- progressUpdate{phase: phase, done: done, total: total}
				},
				OnError: func(stage string, err error) {
					if errors.Is(err, context.Canceled) {
						return
					}
					// sub <- logUpdate{text: fmt.Sprintf("Error in %s: %v", stage, err)}
				},
				OnComplete: func(err error) {
					if err != nil {
						sub <- errorUpdate{err: err}
					} else {
						sub <- completeUpdate{}
					}
					close(sub)
				},
			}
			// Run translation
			_ = runner.RunTranslationWithConfig(ctx, input, output, &runnerCfg, cb)
		}()
		return nil
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen()) // Use AltScreen for better UI
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
