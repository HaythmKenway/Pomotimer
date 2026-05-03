# PomoTimer

A high-performance, TUI-based Pomodoro timer built with Go and the Bubble Tea framework. Designed for users who live in the terminal and want a distraction-free, fullscreen productivity tool.

![License](https://img.shields.io/badge/license-MIT-blue.svg)

## 🚀 Features
- **Fullscreen TUI**: Immersive interface using the Alternate Screen Buffer (no flickering).
- **Persistent Logging**: Automatically saves session data to a local SQLite database (`tracker.db`).
- **Activity Tracking**: Prompted recording of "What did you do?" at the end of focus sessions.
- **Thought Log**: Quickly capture notes into `~/user.log` without leaving the timer.
- **Desktop Integration**: Native Linux notifications via `notify-send`.
- **AI Scoring**: Optional productivity scoring via local [Ollama](https://ollama.com/) integration.

## 🛠️ Getting Started
Check out the [Installation Guide](docs/installation.md) for detailed setup instructions.

```bash
# Quick Start
go run .
```

## 📖 Documentation
- [Configuration](docs/configuration.md)
- [Usage & Keybindings](docs/usage.md)
- [Installation](docs/installation.md)

## 📜 License
MIT
