import { Inter } from "next/font/google";
import StackList from "@/components/ui/stack-list";
import { ThemeToggle } from "@/components/ui/theme-toggle";

const inter = Inter({ subsets: ["latin"] });

export default function Home() {
  return (
    <div className={`flex min-h-screen flex-col ${inter.className}`}>
      <header className="bg-background py-4 px-6 shadow-md">
        <div className="flex justify-between items-center">
          <h1 className="text-xl font-bold">Bucket Manager</h1>
          <ThemeToggle />
        </div>
      </header>
      <main className="flex-grow container mx-auto p-6">
        {/* Stack listing component */}
        <StackList />
      </main>
      <footer className="bg-background py-4 px-6 text-center text-sm text-muted-foreground">
        &copy; 2025 Bucket Manager
      </footer>
    </div>
  );
}
