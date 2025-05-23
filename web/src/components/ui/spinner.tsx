import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

interface SpinnerProps {
  size?: "sm" | "md" | "lg";
  className?: string;
  text?: string;
  centered?: boolean;
}

export function Spinner({
  size = "md",
  className,
  text,
  centered = false,
}: SpinnerProps) {
  const sizeClasses = {
    sm: "h-3 w-3",
    md: "h-5 w-5",
    lg: "h-8 w-8",
  };

  const spinnerElement = (
    <Loader2
      className={cn(
        "animate-spin text-primary",
        sizeClasses[size],
        className
      )}
    />
  );

  if (centered || text) {
    return (
      <div className={cn(
        "flex items-center",
        centered && "justify-center",
        text ? "gap-2" : ""
      )}>
        {spinnerElement}
        {text && <span className={cn(
          "text-foreground",
          size === "lg" && "text-lg",
          size === "sm" && "text-sm"
        )}>{text}</span>}
      </div>
    );
  }

  return spinnerElement;
}
