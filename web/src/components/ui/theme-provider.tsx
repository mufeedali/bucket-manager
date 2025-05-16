"use client"

import * as React from "react"
import { ThemeProvider as NextThemesProvider, type ThemeProviderProps as NextThemesProviderProps } from "next-themes"

type ThemeProviderProps = Omit<NextThemesProviderProps, 'attribute'> & {
  children: React.ReactNode
  attribute?: 'class' | 'data-theme'
}

export function ThemeProvider({ children, ...props }: ThemeProviderProps) {
  return <NextThemesProvider {...props}>{children}</NextThemesProvider>
}
