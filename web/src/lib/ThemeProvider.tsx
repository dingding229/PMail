import { useEffect, useState, type ReactNode } from 'react'
import { getRawStorage, setRawStorage } from './storage'
import { ThemeContext, type ResolvedTheme, type ThemeMode } from './theme'

function getSystemTheme(): ResolvedTheme {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(() => {
    const stored = getRawStorage('theme')
    if (stored === 'light' || stored === 'dark' || stored === 'system') return stored
    return 'system'
  })

  const [systemTheme, setSystemTheme] = useState<ResolvedTheme>(getSystemTheme)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystemTheme(mq.matches ? 'dark' : 'light')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const resolved: ResolvedTheme = mode === 'system' ? systemTheme : mode

  useEffect(() => {
    const root = document.documentElement
    const nextDark = resolved === 'dark'
    const prevDark = root.classList.contains('dark')
    if (prevDark !== nextDark) {
      root.classList.add('theme-switching')
      root.classList.toggle('dark', nextDark)
      // Force synchronous style recalculation so all surfaces, buttons, and nav items switch atomically
      void window.getComputedStyle(root).opacity
      const raf = window.requestAnimationFrame(() => {
        root.classList.remove('theme-switching')
      })
      return () => {
        window.cancelAnimationFrame(raf)
        root.classList.remove('theme-switching')
      }
    }
    root.classList.toggle('dark', nextDark)
  }, [resolved])

  const setMode = (next: ThemeMode) => {
    setModeState(next)
    setRawStorage('theme', next)
  }

  return (
    <ThemeContext.Provider value={{ mode, resolved, setMode }}>
      {children}
    </ThemeContext.Provider>
  )
}
