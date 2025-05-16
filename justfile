# Default task
default: install

# Build the bm binary and install it to ~/.local/bin
install: build-web
    @echo "Building bm binary..."
    go build -o bm ./cmd/bm
    @echo "Ensuring ~/.local/bin exists..."
    mkdir -p ~/.local/bin
    @echo "Copying bm to ~/.local/bin..."
    cp bm ~/.local/bin/
    @echo "Cleaning up artifacts..."
    rm -f bm
    find internal/web/assets -mindepth 1 -not -name ".gitkeep" -delete
    @echo "Installation complete. 'bm' is now available in ~/.local/bin"

# Build the web UI
build-web:
    @echo "Building web UI..."
    cd web && bun install && bun run build
    @echo "Copying web UI build output to embedded directory..."
    mkdir -p internal/web/assets
    cp -r web/out/* internal/web/assets/

# Simple build task (optional)
build: build-web
    @echo "Building bm binary..."
    go build -o bm ./cmd/bm
    @echo "Build complete: ./bm"

# Clean build artifacts
clean:
    @echo "Cleaning build artifacts..."
    rm -f bm
    find internal/web/assets -mindepth 1 -not -name ".gitkeep" -delete
