BIN_DIR := bin
BINARY := $(BIN_DIR)/lumin-engine
LLAMA_BUILD_DIR := vendor/llama.cpp/build
LLAMA_LIB := lib/libllama.a

.PHONY: build clean run install llama

# Default: build everything
build: llama $(BINARY)

# Build llama.cpp as static library
llama:
	@if [ ! -d vendor/llama.cpp ]; then \
		echo "Cloning llama.cpp..."; \
		git clone --depth 1 https://github.com/ggml-org/llama.cpp vendor/llama.cpp; \
	fi
	@echo "Building llama.cpp with CUDA support..."
	@cd vendor/llama.cpp && \
		cmake -B build \
			-DLLAMA_CUDA=ON \
			-DBUILD_SHARED_LIBS=OFF \
			-DCMAKE_BUILD_TYPE=Release && \
		cmake --build build --config Release -j$$(nproc)
	@cp vendor/llama.cpp/build/src/libllama.a lib/libllama.a
	@cp vendor/llama.cpp/llama.h lib/llama.h
	@echo "✓ libllama.a built"

# Build Go binary
$(BINARY): llama
	@mkdir -p $(BIN_DIR)
	@echo "Building lumin-engine..."
	@CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(PWD)/lib" \
		CGO_LDFLAGS="-L$(PWD)/lib -lllama" \
		go build -o $(BINARY) ./cmd/lumin-engine
	@echo "✓ $(BINARY) built"

run: build
	./$(BINARY) -socket /tmp/lumin-engine.sock

install: build
	@echo "Installing..."
	install -d /usr/lib/lumin
	install -m755 $(BINARY) /usr/lib/lumin/lumin-engine
	install -d /etc/lumin
	install -m644 lumin-engine.service /etc/systemd/user/
	systemctl --user daemon-reload
	systemctl --user enable lumin-engine
	@echo "✓ Installation complete"

clean:
	rm -rf $(BIN_DIR)
	rm -f lib/libllama.a lib/llama.h
	rm -rf vendor/llama.cpp/build
