This is the web UI for Bucket Manager, built with:

- [Next.js](https://nextjs.org) - React framework
- [React.js](https://reactjs.org) - UI library
- [shadcn/ui](https://ui.shadcn.com/) - UI component library
- [Bun](https://bun.sh) - JavaScript runtime & package manager

The UI communicates with a Go REST backend to manage Podman Compose stacks.

## Getting Started

First, install dependencies:

```bash
bun install
```

Then, run the development server:

```bash
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Building for Production

To build the web UI for production use:

```bash
just install
```

The web UI is accessed through the `bm serve` command which starts the Go backend server with this UI embedded.
