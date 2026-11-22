import { createContext, useContext } from 'react'

export type ListDensity = 'compact' | 'comfortable' | 'spacious'
export type ReadingPanePosition = 'right' | 'bottom' | 'off'

export interface PreferencesContextValue {
  listDensity: ListDensity
  readingPanePosition: ReadingPanePosition
  setListDensity: (density: ListDensity) => void
  setReadingPanePosition: (pos: ReadingPanePosition) => void
}

export const PreferencesContext = createContext<PreferencesContextValue | null>(null)

export function usePreferences(): PreferencesContextValue {
  const ctx = useContext(PreferencesContext)
  if (!ctx) throw new Error('usePreferences must be used inside PreferencesProvider')
  return ctx
}
