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
	@echo "Building llama.cpp (auto CUDA/CPU)..."
	@CUDA_FLAG="-DGGML_CUDA=OFF"; \
	if command -v nvcc >/dev/null 2>&1; then \
		CUDA_FLAG="-DGGML_CUDA=ON"; \
		echo "CUDA toolkit detected, enabling GGML_CUDA"; \
	else \
		echo "CUDA toolkit not found, building CPU backend"; \
	fi; \
	cd vendor/llama.cpp && \
	cmake -B build \
		$$CUDA_FLAG \
		-DBUILD_SHARED_LIBS=OFF \
		-DCMAKE_BUILD_TYPE=Release && \
	cmake --build build --config Release -j$$(nproc)
	@cp vendor/llama.cpp/build/src/libllama.a lib/libllama.a
	@echo "✓ libllama.a built"

# Build Go binary
$(BINARY): llama
	@mkdir -p $(BIN_DIR)
	@echo "Building lumin-engine..."
	@CGO_ENABLED=1 \
		CGO_CFLAGS="-I$(PWD)/vendor/llama.cpp/include -I$(PWD)/vendor/llama.cpp/ggml/include" \
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
