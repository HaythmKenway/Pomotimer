package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Step struct {
	Name              string
	Duration          time.Duration
	RequiresRecording bool
}

type sessionState int

const (
	timerView sessionState = iota
	historyView
	configView
	inputView
	scoreView
)

type model struct {
	state       sessionState
	steps       []Step
	currentStep int
	timer       timer.Model
	progress    progress.Model
	db          *sql.DB
	startTime   time.Time
	quitting    bool
	history     []LogEntry

	// Log monitoring
	userLog []string

	// Config mode fields
	inputs     []textinput.Model
	focusIndex int
	err        string

	// Direct logging
	logInput textinput.Model

	// Scoring
	scores   map[string]int
	isScoring bool
}

type logMsg []string
type systemInactiveTickMsg struct{}

func isSystemActive() bool {
	// 1. Check if screen is locked (GNOME)
	// We check for DISPLAY to ensure we are in a graphical session
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd := exec.Command("gdbus", "call", "--session", "--dest", "org.gnome.ScreenSaver", "--object-path", "/org/gnome/ScreenSaver", "--method", "org.gnome.ScreenSaver.GetActive")
		out, err := cmd.Output()
		if err == nil && strings.Contains(string(out), "true") {
			return false
		}
	}

	// 2. Check session state via loginctl
	// This helps detect "logged off" or session closing
	cmd := exec.Command("loginctl", "show-user", os.Getenv("USER"), "-p", "State")
	out, err := cmd.Output()
	if err == nil {
		state := string(out)
		if strings.Contains(state, "State=closing") || strings.Contains(state, "State=lingering") {
			return false
		}
		if !strings.Contains(state, "active") && !strings.Contains(state, "online") {
			return false
		}
	}

	return true
}

func pollLog() tea.Cmd {
	return tea.Tick(time.Second*5, func(t time.Time) tea.Msg {
		content, err := os.ReadFile(os.Getenv("HOME") + "/user.log")
		if err != nil {
			return logMsg{"Error reading log"}
		}
		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		if len(lines) > 12 {
			lines = lines[len(lines)-12:]
		}
		return logMsg(lines)
	})
}

func initialModel(db *sql.DB, steps []Step) model {
	tiName := textinput.New()
	tiName.Placeholder = "Step Name (e.g. Documentation)"
	tiName.Focus()
	tiName.CharLimit = 32
	tiName.Width = 30

	tiDur := textinput.New()
	tiDur.Placeholder = "Duration (minutes)"
	tiDur.CharLimit = 3
	tiDur.Width = 10

	tiLog := textinput.New()
	tiLog.Placeholder = "What are you thinking? (press Enter to log)"
	tiLog.CharLimit = 120
	tiLog.Width = 60

	prog := progress.New(progress.WithDefaultGradient())
	prog.Width = 30

	// Send a test notification to ensure it works
	notify("PomoTimer Started!", "low")

	return model{
		state:       timerView,
		steps:       steps,
		currentStep: 0,
		timer:       timer.NewWithInterval(steps[0].Duration, time.Second),
		progress:    prog,
		db:          db,
		startTime:   time.Now(),
		inputs:      []textinput.Model{tiName, tiDur},
		userLog:     []string{"Loading logs..."},
		logInput:    tiLog,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.timer.Init(), pollLog())
}

func askWhatIDid(stepName string) {
	cmd := exec.Command("zenity", "--entry", "--title=PomoTimer Recording", "--text=What did you do during: "+stepName+"?")
	out, err := cmd.Output()
	if err == nil {
		thought := strings.TrimSpace(string(out))
		if thought != "" {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			line := fmt.Sprintf("%s [%s] %s\n", timestamp, stepName, thought)
			f, err := os.OpenFile(os.Getenv("HOME")+"/user.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(line)
				f.Close()
			}
		}
	}
}

type scoreMsg map[string]int

func getOllamaScore(logs []string) tea.Cmd {
	return func() tea.Msg {
		if len(logs) == 0 {
			return scoreMsg{"Productivity": 0, "Focus": 0, "Creativity": 0}
		}

		prompt := "Based on the following activity logs, provide a score from 0 to 100 for 'Productivity', 'Focus', and 'Creativity'. Respond ONLY with a JSON object like {\"Productivity\": 80, \"Focus\": 70, \"Creativity\": 90}. Logs:\n" + strings.Join(logs, "\n")

		payload := map[string]interface{}{
			"model":  "llama3.2:latest",
			"prompt": prompt,
			"stream": false,
			"format": "json",
		}

		jsonData, _ := json.Marshal(payload)
		resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return scoreMsg{"Error": 0}
		}
		defer resp.Body.Close()

		var result struct {
			Response string `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return scoreMsg{"Error": 0}
		}

		var scores map[string]int
		if err := json.Unmarshal([]byte(result.Response), &scores); err != nil {
			return scoreMsg{"Error": 0}
		}

		return scoreMsg(scores)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scoreMsg:
		m.isScoring = false
		m.scores = msg
		return m, nil
	case logMsg:
		m.userLog = msg
		return m, pollLog()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}

		switch m.state {
		case timerView:
			switch msg.String() {
			case "s":
				m.state = scoreView
				m.isScoring = true
				return m, getOllamaScore(m.userLog)
			case "n":
				if m.steps[m.currentStep].RequiresRecording {
					go askWhatIDid(m.steps[m.currentStep].Name)
				}
				return m.nextStep("normal")
			case "r":
				m.timer = timer.NewWithInterval(m.steps[m.currentStep].Duration, time.Second)
				m.startTime = time.Now()
				return m, m.timer.Init()
			case "h":
				m.state = historyView
				m.history = getHistory(m.db)
				return m, nil
			case "c":
				m.state = configView
				return m, nil
			case "i":
				m.state = inputView
				m.logInput.Focus()
				return m, nil
			}

		case scoreView:
			if msg.String() == "s" || msg.String() == "esc" {
				m.state = timerView
			}
			return m, nil

		case historyView:
			if msg.String() == "h" || msg.String() == "esc" {
				m.state = timerView
			}
			return m, nil

		case inputView:
			switch msg.String() {
			case "esc":
				m.state = timerView
				m.logInput.Blur()
				m.logInput.SetValue("")
				return m, nil
			case "enter":
				thought := m.logInput.Value()
				if thought != "" {
					// Log to ~/user.log
					timestamp := time.Now().Format("2006-01-02 15:04:05")
					line := fmt.Sprintf("%s %s\n", timestamp, thought)
					f, err := os.OpenFile(os.Getenv("HOME")+"/user.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						f.WriteString(line)
						f.Close()
					}
				}
				m.state = timerView
				m.logInput.Blur()
				m.logInput.SetValue("")
				return m, pollLog() // Refresh log immediately
			}
			var cmd tea.Cmd
			m.logInput, cmd = m.logInput.Update(msg)
			return m, cmd

		case configView:
			switch msg.String() {
			case "esc":
				m.state = timerView
				return m, nil
			case "tab", "shift+tab", "enter", "up", "down":
				s := msg.String()

				if s == "enter" && m.focusIndex == len(m.inputs) {
					// Add step logic
					name := m.inputs[0].Value()
					durStr := m.inputs[1].Value()
					dur, err := strconv.Atoi(durStr)
					if err != nil || dur <= 0 {
						m.err = "Invalid duration"
						return m, nil
					}
					if name == "" {
						m.err = "Name cannot be empty"
						return m, nil
					}

					newStep := Step{Name: name, Duration: time.Duration(dur) * time.Minute}
					m.steps = append(m.steps, newStep)
					saveSteps(m.steps)
					
					m.inputs[0].SetValue("")
					m.inputs[1].SetValue("")
					m.focusIndex = 0
					m.inputs[0].Focus()
					m.inputs[1].Blur()
					m.err = ""
					return m, nil
				}

				if s == "up" || s == "shift+tab" {
					m.focusIndex--
				} else {
					m.focusIndex++
				}

				if m.focusIndex > len(m.inputs) {
					m.focusIndex = 0
				} else if m.focusIndex < 0 {
					m.focusIndex = len(m.inputs)
				}

				cmds := make([]tea.Cmd, len(m.inputs))
				for i := 0; i <= len(m.inputs)-1; i++ {
					if i == m.focusIndex {
						cmds[i] = m.inputs[i].Focus()
						continue
					}
					m.inputs[i].Blur()
				}
				return m, tea.Batch(cmds...)

			case "d":
				if len(m.steps) > 1 {
					m.steps = m.steps[:len(m.steps)-1]
					saveSteps(m.steps)
					if m.currentStep >= len(m.steps) {
						m.currentStep = 0
						m.timer = timer.NewWithInterval(m.steps[0].Duration, time.Second)
						return m, m.timer.Init()
					}
				}
				return m, nil
			}

			var cmds []tea.Cmd
			for i := range m.inputs {
				var cmd tea.Cmd
				m.inputs[i], cmd = m.inputs[i].Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

	case systemInactiveTickMsg:
		if !isSystemActive() {
			return m, func() tea.Msg {
				time.Sleep(time.Second)
				return systemInactiveTickMsg{}
			}
		}
		// System active again, resume timer
		return m, m.timer.Init()

	case timer.TickMsg:
		if !isSystemActive() {
			return m, func() tea.Msg {
				time.Sleep(time.Second)
				return systemInactiveTickMsg{}
			}
		}
		var cmd tea.Cmd
		m.timer, cmd = m.timer.Update(msg)
		
		// Update progress bar
		percent := 1.0 - float64(m.timer.Timeout)/float64(m.steps[m.currentStep].Duration)
		progressCmd := m.progress.SetPercent(percent)
		
		return m, tea.Batch(cmd, progressCmd)

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case timer.StartStopMsg:
		var cmd tea.Cmd
		m.timer, cmd = m.timer.Update(msg)
		return m, cmd

	case timer.TimeoutMsg:
		logSession(m.db, m.steps[m.currentStep].Name, int(m.steps[m.currentStep].Duration.Minutes()), m.startTime)
		if m.steps[m.currentStep].RequiresRecording {
			go askWhatIDid(m.steps[m.currentStep].Name)
		}
		return m.nextStep("critical")
	}

	return m, nil
}

func (m model) nextStep(urgency string) (model, tea.Cmd) {
	prevStep := m.steps[m.currentStep].Name
	m.currentStep = (m.currentStep + 1) % len(m.steps)
	m.timer = timer.NewWithInterval(m.steps[m.currentStep].Duration, time.Second)
	m.startTime = time.Now()
	notify(fmt.Sprintf("Next: %s (Finished: %s)", m.steps[m.currentStep].Name, prevStep), urgency)
	return m, m.timer.Init()
}

func saveSteps(steps []Step) {
	configSteps := make([]ConfigStep, len(steps))
	for i, s := range steps {
		configSteps[i] = ConfigStep{
			Name:              s.Name,
			Duration:          int(s.Duration.Minutes()),
			RequiresRecording: s.RequiresRecording,
		}
	}
	data, _ := os.Create("config.json")
	defer data.Close()

	encoder := json.NewEncoder(data)
	encoder.SetIndent("", "  ")
	encoder.Encode(Config{Steps: configSteps})
}


func notify(message string, urgency string) {
	// Robust notification for Ubuntu
	// Using --icon=clock-symbolic or similar if available
	// Use a synchronous hint to replace existing notifications and avoid flooding
	exec.Command("notify-send", "-u", urgency, "-a", "PomoTimer", "-h", "string:x-canonical-private-synchronous:pomo-timer", "PomoTimer", message).Run()
}

func getClockGlyph(percent float64) string {
	glyphs := []string{
		"🕛", "🕧", "🕐", "🕜", "🕑", "🕝", "🕒", "🕞", "🕓", "🕟", "🕔", "🕠",
		"🕕", "🕡", "🕖", "🕢", "🕗", "🕣", "🕘", "🕤", "🕙", "🕥", "🕚", "🕦",
	}
	// Reverse percentage because we want to show time *remaining* relative to a full clock
	// 0% elapsed = 100% remaining = 🕛
	// 50% elapsed = 50% remaining = 🕕
	idx := int(percent * float64(len(glyphs)))
	if idx >= len(glyphs) {
		idx = len(glyphs) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return glyphs[idx]
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1).
			MarginTop(1)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3C3836")).
			Padding(1, 2)

	activePanelStyle = panelStyle.Copy().
			BorderForeground(lipgloss.Color("#7D56F4"))

	timerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true).
			Align(lipgloss.Center).
			Width(25)

	glyphStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			MarginRight(1)

	stepNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true).
			MarginBottom(1)

	listStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A89984"))

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).MarginTop(1).MarginLeft(2)
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	activeTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Underline(true)

	logTimeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C6F64"))
	logContentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D79921"))
	logURLStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#458588")).Underline(true)
)

func colorizeLog(line string) string {
	if line == "" {
		return ""
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return line
	}

	timestamp := logTimeStyle.Render(parts[0] + " " + parts[1])
	content := parts[2]

	// Simple URL highlighting
	if strings.Contains(content, "http") {
		content = strings.ReplaceAll(content, "http", "~~http")
		parts := strings.Split(content, "~~")
		newContent := ""
		for _, p := range parts {
			if strings.HasPrefix(p, "http") {
				end := strings.IndexAny(p, " \n\t")
				if end == -1 {
					newContent += logURLStyle.Render(p)
				} else {
					newContent += logURLStyle.Render(p[:end]) + p[end:]
				}
			} else {
				newContent += p
			}
		}
		content = newContent
	}

	return timestamp + " " + logContentStyle.Render(content)
}

func (m model) getTodayStats() map[string]int {
	logs := getHistory(m.db)
	stats := make(map[string]int)
	today := time.Now().Format("2006-01-02")
	
	// Stats from DB
	for _, l := range logs {
		if l.StartTime.Format("2006-01-02") == today {
			stats[l.StepName] += l.Duration
		}
	}

	// Reactive: Add currently running session (in minutes)
	elapsed := time.Since(m.startTime).Minutes()
	stats[m.steps[m.currentStep].Name] += int(elapsed)

	return stats
}

func (m model) View() string {
	if m.quitting {
		return "\n  Exiting PomoTimer...\n\n"
	}

	switch m.state {
	case historyView:
		s := titleStyle.Render(" History ") + "\n\n"
		for _, entry := range m.history {
			s += fmt.Sprintf("  [%s] %s: %d mins\n", entry.StartTime.Format("15:04"), entry.StepName, entry.Duration)
		}
		if len(m.history) == 0 {
			s += "  No history yet.\n"
		}
		s += helpStyle.Render("\nh: back to timer")
		return s

	case configView:
		s := titleStyle.Render(" Configuration ") + "\n\n"
		
		s += "  Current Workflow:\n"
		for i, step := range m.steps {
			if i == m.currentStep {
				s += fmt.Sprintf("  -> %s (%v)\n", step.Name, step.Duration)
			} else {
				s += fmt.Sprintf("     %s (%v)\n", step.Name, step.Duration)
			}
		}
		
		s += "\n  Add New Step:\n"
		for i := range m.inputs {
			s += "  " + m.inputs[i].View() + "\n"
		}

		button := "[ Submit ]"
		if m.focusIndex == len(m.inputs) {
			button = activeTabStyle.Render("[ Submit ]")
		}
		s += "\n  " + button + "\n"

		if m.err != "" {
			s += "\n  " + errorStyle.Render(m.err) + "\n"
		}

		s += helpStyle.Render("\nesc: back • tab: focus • d: delete last step")
		return s

	case inputView:
		s := titleStyle.Render(" New Log Entry ") + "\n\n"
		s += "  Format: [YYYY-MM-DD HH:MM:SS] <your thought>\n\n"
		s += "  " + m.logInput.View() + "\n"
		s += helpStyle.Render("\nenter: save log • esc: cancel")
		return s

	default:
		header := lipgloss.NewStyle().MarginLeft(2).Render(titleStyle.Render(" PomoTimer Dashboard "))

		// Left Panel: Active Timer + Progress + Glyph
		percent := 1.0 - float64(m.timer.Timeout)/float64(m.steps[m.currentStep].Duration)
		glyph := glyphStyle.Render(getClockGlyph(percent))
		
		timerView := lipgloss.JoinHorizontal(lipgloss.Center, glyph, " ", timerStyle.Render(m.timer.View()))

		activeContent := stepNameStyle.Render(m.steps[m.currentStep].Name) + "\n\n"
		activeContent += timerView + "\n\n"
		activeContent += m.progress.View()
		
		leftPanel := activePanelStyle.Render(activeContent)

		// Middle Panel: Up Next & Live Stats
		midContent := stepNameStyle.Copy().Foreground(lipgloss.Color("#FAFAFA")).Render("Up Next") + "\n"
		for i := 1; i < len(m.steps); i++ {
			idx := (m.currentStep + i) % len(m.steps)
			midContent += listStyle.Render(fmt.Sprintf("• %s (%v)", m.steps[idx].Name, m.steps[idx].Duration)) + "\n"
		}

		midContent += "\n" + stepNameStyle.Copy().Foreground(lipgloss.Color("#FAFAFA")).Render("Live Today (mins)") + "\n"
		stats := m.getTodayStats()
		for name, mins := range stats {
			midContent += listStyle.Render(fmt.Sprintf("• %s: %dm", name, mins)) + "\n"
		}

		midPanel := panelStyle.Render(midContent)

		// Right Panel: User Log
		logView := stepNameStyle.Copy().Foreground(lipgloss.Color("#FAFAFA")).Render("Thought Log (~/user.log)") + "\n"
		for _, line := range m.userLog {
			if line != "" {
				logView += colorizeLog(line) + "\n"
			}
		}
		rightPanel := panelStyle.Copy().Width(60).Height(14).Render(logView)

		dashboard := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", midPanel, "  ", rightPanel)

		footer := helpStyle.Render("q: quit • n: next • r: reset • h: history • c: config • i: log thought • s: score")

		return header + "\n\n" + lipgloss.NewStyle().MarginLeft(2).Render(dashboard) + "\n\n" + footer + "\n"
	}
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	steps, err := loadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	m := initialModel(db, steps)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
