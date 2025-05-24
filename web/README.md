# Bucket Manager Web UI

The web UI for Bucket Manager, providing a modern graphical interface to manage compose stacks. This React-based application is embedded into the Go binary at build time to create a single, self-contained executable that includes the entire web interface.

## Technology Stack

- [Next.js](https://nextjs.org) - React framework
- [React.js](https://reactjs.org) - UI library
- [Tailwind CSS](https://tailwindcss.com) - Utility-first CSS framework
- [shadcn/ui](https://ui.shadcn.com/) - UI component library
- [Bun](https://bun.sh) - JavaScript runtime & package manager

## Development

```bash
# Install dependencies
bun install

# Start development server
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

## Integration with Go Backend

The web UI is embedded into the Go binary during build:

When `just build-web` or `just install` is run:
   - Web UI is built with `bun run build`
   - Build output is copied to `internal/web/assets` in the main project
   - Go binary embeds these assets and serves them via `bm serve` command

## Production Use

End users don't need to build the web UI separately. The complete web UI is embedded in the main `bm` binary and accessed by running:

```bash
bm serve
```

This starts a web server on port 8080 that serves both the UI and the API endpoints.