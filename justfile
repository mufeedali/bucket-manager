# Default task
default: install

# Build the bm binary and install it to ~/.local/bin
install:
    @echo "Building bm binary..."
    go build -o bm ./cmd/bm
    @echo "Ensuring ~/.local/bin exists..."
    mkdir -p ~/.local/bin
    @echo "Copying bm to ~/.local/bin..."
    cp bm ~/.local/bin/
    @echo "Cleaning up local binary..."
    rm bm
    @echo "Installation complete. 'bm' is now available in ~/.local/bin"

# Simple build task (optional)
build:
    @echo "Building bm binary..."
    go build -o bm ./cmd/bm
    @echo "Build complete: ./bm"

# Clean build artifacts
clean:
    @echo "Cleaning build artifacts..."
    rm -f bm
