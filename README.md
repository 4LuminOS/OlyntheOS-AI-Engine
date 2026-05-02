# Lumin-engine

`lumin-engine` is a Go-based local assistant engine that exposes a Unix socket API, manages model lifecycle, and routes tool calls through a permissions layer. It runs as a systemd service, listens on a Unix socket, and bridges natural language requests to safe OS actions.

---

## How a user message becomes a system action

```
User types message
    ↓
IPC socket receives JSON request
    ↓ cmd/lumin-engine/main.go
Tokenize text → integers
    ↓ internal/inference/tokenizer.go (loaded from GGUF)
Build context window with conversation history
    ↓ internal/context/manager.go (sliding window with truncation)
Run inference via llama.cpp
    ↓ internal/inference/llama_cgo.go (CGo bindings)
Detokenize output → text
    ↓ internal/inference/tokenizer.go
Stream tokens to client + parse tool calls
    ↓ internal/tools/parser.go (state machine, detects [[tool:...]])
Permission check for each tool call
    ↓ internal/permissions/policy.go (capability flags, path allowlist)
Execute OS action via sandboxed Go wrapper
    ↓ internal/tools/executor.go (fs_read, exec_safe, notify, etc.)
Audit log the result
    ↓ internal/permissions/audit.go (timestamped action record)
Return result to client
```

---

## The 6 core components

### 1. Inference core — llama.cpp via CGo
**Files**: `internal/inference/llama_cgo.go`, `internal/inference/model.go`, `internal/inference/generate.go`

Loads GGUF model weights, runs the forward pass (attention + feed-forward layers), outputs probability distributions over tokens. The actual neural network math. You bind to llama.cpp (already battle-tested) and get CUDA/ROCm/Metal/CPU backend selection for free.

### 2. Tokenizer — text ↔ numbers
**Files**: `internal/inference/tokenizer.go`

Converts user text → integer token IDs before inference, and output token IDs → text. Each model has its own vocabulary (Qwen3 uses BPE, Gemma uses SentencePiece). The real tokenizer is embedded in the GGUF file itself.

### 3. Context window manager — conversation memory
**Files**: `internal/context/manager.go`, `internal/context/template.go`

Builds the full prompt, tracks conversation history, injects system prompt, truncates old messages when the window fills up. This is what makes the AI "remember" what you said. Models can only "see" a fixed number of tokens at once (e.g. 32,768 for Qwen3 8B).

### 4. Tool calling parser — text → structured action
**Files**: `internal/tools/parser.go`

Watches the output stream, detects `[[tool:...]]` blocks mid-stream (without buffering the whole response), extracts JSON, routes to the right Go wrapper function. Modern LLMs emit structured JSON when they want to take an action. This is the bridge between "AI thinking" and "OS doing".

### 5. Hardware abstraction layer — GPU/CPU routing
**Files**: `internal/hardware/probe.go`, `internal/hardware/recommend.go`, `internal/hardware/watchdog.go`

Probes the system at startup to find the best compute backend. If an Nvidia GPU with enough VRAM is found, it offloads layers to GPU. Falls back to CPU. Enforces 80% RAM ceiling and handles suspend/resume. You want inference to be fast, but not crash the system.

### 6. IPC daemon — the Unix socket server
**Files**: `internal/ipc/server.go`, `internal/ipc/handler.go`, `internal/ipc/protocol.go`, `cmd/lumin-engine/main.go`

The Go daemon that runs as a systemd service. Listens on a Unix socket at `/run/lumin/engine.sock`. Any app (desktop widget, panel applet, terminal) connects and sends/receives JSON-RPC 2.0 messages. This is the interface between the rest of LuminOS and the AI engine. Nothing touches the system except through this.

---

## Permission sandbox

Every tool has a capability flag. Users grant capabilities per-model via `~/.config/lumin/permissions.toml`, and can scope them to specific paths or D-Bus methods. Revocation takes effect immediately, no restart needed.

### Capability map

| Capability | Risk | Behavior |
|------------|------|----------|
| `fs.read` | Low | Read files/dirs. Scoped to allowlisted paths only (e.g. `~/Documents`). Never `/` or `/etc` without explicit grant. |
| `fs.write` | Medium | Write/create files. Requires path allowlist. Writes to `/tmp` by default. Destructive ops (delete, overwrite) need separate flag. |
| `plasma.theme` | Low | Change KDE theme, panel config, wallpaper via D-Bus. Fully reversible. |
| `plasma.windows` | Low | Read window list, focus, minimize, tile. Write requires explicit grant. |
| `net.fetch` | Medium | HTTP GET to external URLs. Domain allowlist enforced. |
| `exec.run` | **High** | Run shell commands. Off by default. Allowlisted commands only. |
| `notify.send` | Low | Send desktop notifications. Cannot impersonate system. |
| `clipboard` | Medium | Read/write clipboard. Separate flags. |

---

## Project layout

### Entry point
- `cmd/lumin-engine/main.go` — application entrypoint; loads config, initializes model and permissions, starts socket server.
- `lumin-engine.service` — systemd user service file.

### Configuration
- `internal/config/config.go` — loads `/etc/lumin/engine.toml` (socket path, permissions, audit log, context size).

### Inference
- `internal/inference/llama_cgo.go` — **real** CGo bindings to llama.cpp C API. Loads models, runs inference, tokenization.
- `internal/inference/model.go` — model state, load/unload/suspend lifecycle.
- `internal/inference/generate.go` — generation wrapper with sampling options.
- `internal/inference/tokenizer.go` — thin wrappers around the loaded model's tokenizer and detokenizer.
- `internal/inference/llama_stub.go` — non-CGo fallback.

### Conversation context
- `internal/context/manager.go` — tracks message history, auto-trims by token budget.
- `internal/context/template.go` — renders prompts (Qwen3, Gemma, Llama).

### Tools
- `internal/tools/parser.go` — **streaming state machine** detects `[[tool:...]]` blocks mid-stream.
- `internal/tools/executor.go` — dispatches tool calls, checks permissions, logs actions.
- `internal/tools/fs.go` — file read/write helpers.
- `internal/tools/exec.go` — restricted command execution (allowlist only).
- `internal/tools/notify.go` — notification wrapper.
- `internal/tools/plasma.go` — KDE Plasma D-Bus wrapper.
- `internal/tools/web.go` — HTTP GET helper (timeout + size limit).

### Permissions
- `internal/permissions/policy.go` — loads, saves, evaluates permissions from TOML. Capability flags, path/command allowlists.
- `internal/permissions/audit.go` — writes timestamped audit log entries.

### Hardware
- `internal/hardware/probe.go` — system memory/GPU detection.
- `internal/hardware/recommend.go` — recommends model size based on RAM.
- `internal/hardware/watchdog.go` — background RAM monitor, triggers suspend at 80%.

### IPC
- `internal/ipc/server.go` — Unix socket listener with clean shutdown.
- `internal/ipc/handler.go` — JSON-RPC 2.0 dispatcher: `health`, `generate`, `model.load`, `model.unload`, `tool.call`.
- `internal/ipc/protocol.go` — request/response types.

### Native assets
These native headers and libraries are NOT committed into the repository. Run the build target below to clone and compile `llama.cpp` locally — that produces `lib/llama.h` and `lib/libllama.a` under `lib/`.

IMPORTANT: Do not add prebuilt `libllama.a` or `llama.h` to the repo. Always build on the target machine (toolchain, GPU drivers, and ABI vary).

### Build files
- `go.mod` — Go module definition (stdlib only, except CGo to llama.cpp).
- `go.sum` — dependency checksums.
- `Makefile` — full build pipeline: clone llama.cpp, compile with CUDA, link Go binary.

---

## Build

```bash
make build
```

**What it does:**
1. Clones `llama.cpp` from GitHub (if not present)
2. Builds with CUDA support (auto-detects; falls back to CPU-only)
3. Copies `libllama.a` and `llama.h` to `lib/`
4. Builds Go binary with CGo enabled
5. Produces `bin/lumin-engine`

**Requirements:**
- `gcc` or `clang` (C compiler)
- `cmake` (for llama.cpp build system)
- Go 1.22+
- Optional: CUDA toolkit (for GPU support)

## Run

```bash
make run
````markdown
# lumin-engine

One consolidated document: this `README.md` contains the full project overview, architecture, build instructions, and the implementation/verification summaries previously spread across multiple Markdown files.

If you prefer the older, separate documents, their material is included below — the repository now maintains a single authoritative `README.md` to keep docs centralized.

---

## Quick Overview

`lumin-engine` is a Go-based local assistant engine that exposes a Unix socket API, manages model lifecycle, and routes tool calls through a permissions sandbox. It supports systemd socket activation, model management, a Qt/QML control panel, and model-backed tokenization via `llama.cpp` (GGUF models).

Core flow:

User message → IPC socket (JSON-RPC) → Tokenize (GGUF) → Context window → Inference (llama.cpp via CGo) → Detokenize → Stream + parse tool calls → Permission check → Execute → Audit log → Return result

---

## Single-file Contents

This file consolidates the following topics:

- Project architecture & components
- Build & run instructions (Makefile + systemd)
- Tokenizer, inference & CGo binding notes
- Systemd socket activation details
- D-Bus (KDE Plasma) bridge and available methods
- Model download manager (CLI) usage
- Qt/QML control panel overview
- Verification checklist and next steps

---

## Project Layout (short)

- `cmd/lumin-engine/` — main daemon (systemd + socket activation)
- `cmd/model-wizard/` — model download manager (HTTP + SHA256)
- `cmd/lumin-control-panel/` — Qt/QML control panel (Kirigami)
- `internal/inference/` — CGo bindings (`llama_cgo.go`), model lifecycle, tokenizer
- `internal/tools/` — tool parser, executor, KDE Plasma D-Bus wrapper
- `internal/ipc/` — Unix socket server + JSON-RPC handler
- `internal/permissions/` — policy, audit logging
- `internal/context/` — prompt templates and sliding-window manager
- `internal/hardware/` — GPU/RAM detection and watchdog
- `contrib/` — systemd socket & service files
- `Makefile` — build pipeline (clones & builds `llama.cpp` locally)

---

## Important notes about native artifacts

Native headers and libraries for `llama.cpp` are intentionally NOT committed to the repository. Run `make build` from the project root to clone and compile `llama.cpp` locally; the build will place `lib/llama.h` and `lib/libllama.a` into `lib/`.

Do not add prebuilt `libllama.a` or `llama.h` to the repo — native toolchains, ABIs and GPU drivers vary across machines and shipping binaries in-repo leads to brittle builds and security concerns.

---

## Build & Run (condensed)

Prereqs: `gcc`/`clang`, `cmake`, Go 1.22+, optional CUDA toolkit for GPU backends.

Build everything:

```bash
cd /path/to/lumin-engine
make build

# Build model wizard
go build -o bin/model-wizard ./cmd/model-wizard

# Optional: GUI (Qt 6 + KF6 required)
mkdir -p build-gui && cd build-gui
cmake ../cmd/lumin-control-panel
make
sudo make install
```

Quick test (local socket):

```bash
# Run daemon for a quick test
./bin/lumin-engine -socket /tmp/lumin-engine.sock &

# Health check via JSON-RPC (example)
printf '%s' '{"jsonrpc":"2.0","id":1,"method":"health"}' | nc -U /tmp/lumin-engine.sock
```

Systemd install (optional):

```bash
sudo cp contrib/lumin.socket /etc/systemd/system/
sudo cp contrib/lumin-engine.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable lumin.socket
```

---

## Key Implementations & Verified Features

- Tokenizer: Real GGUF tokenizer via `llama_tokenize` and `llama_token_to_piece` (not word-split) — `internal/inference/llama_cgo.go` and `internal/inference/tokenizer.go`.
- Inference: Real llama.cpp C API usage via CGo (`#include "llama.h"`).
- Socket activation: `contrib/lumin.socket` + `cmd/lumin-engine/main.go` uses `activation.Listeners()` to inherit systemd socket.
- D-Bus: `internal/tools/plasma.go` uses `github.com/godbus/dbus/v5` to call `org.kde.Plasma.*` methods (SetTheme, SetWallpaper, GetPanelConfig, etc.).
- Model manager: `cmd/model-wizard` performs streaming HTTP downloads with SHA256 verification and atomic rename into `~/.local/share/lumin/models`.
- GUI: `cmd/lumin-control-panel` (Qt/QML + Kirigami) with `IPCClient.qml` for JSON-RPC over Unix socket.

---

## Architecture & Message Flow (summary)

1. User enters prompt in GUI or sends JSON-RPC to Unix socket.
2. Daemon tokenizes prompt using model's vocab (GGUF-backed tokenizer).
3. Context manager composes conversation + system prompt within max token budget.
4. Inference runs via `llama.cpp` (CGo layer) and streams tokens back.
5. Streaming parser (`internal/tools/parser.go`) watches output for `[[tool:...]]` blocks.
6. When tool call detected, permission engine evaluates policy; if allowed, executor runs sandboxed tool and logs audit entry.
7. Results are returned to client.

---

## Verification Checklist (condensed)

- llama_cgo.go compiles with `#include "llama.h"` (0 errors reported in static checks performed during implementation).
- `internal/tools/plasma.go` compiles and uses godbus/dbus.
- `cmd/lumin-engine/main.go` compiles and supports systemd socket activation.
- `internal/ipc/server.go` exposes a `NewServer(listener, handler)` constructor.
- `cmd/model-wizard` performs streaming download + SHA256 verification and atomic rename.
- Qt/QML control panel scaffolding (CMakeLists + QML) present and ready to build.

---

## Next steps (recommended)

1. Run `make build` on your target machine to produce `lib/llama.h` and `lib/libllama.a`.
2. Use `bin/model-wizard` to fetch a small GGUF test model for integration testing.
3. Start the daemon (socket activation or direct) and run the end-to-end GUI → socket → model → tool → permission → execution → response smoke test.

---

## License

MIT

````
