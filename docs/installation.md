# Installation Guide

## Prerequisites
- **Go**: version 1.24 or higher.
- **SQLite3**: Required for the history tracking database.
- **Linux Environment**: Optimized for Linux (uses `notify-send` for notifications and `zenity` for input popups).

## Building from Source
1. Clone the repository:
   ```bash
   git clone git@github.com:HaythmKenway/Pomotimer.git
   cd Pomotimer
   ```
2. Build the binary:
   ```bash
   go build -o pomotimer
   ```
3. Run the application:
   ```bash
   ./pomotimer
   ```
