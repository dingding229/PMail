import { createContext, useContext, useState, type ReactNode } from 'react'
import { getRawStorage, setRawStorage } from './storage'

export type ListDensity = 'compact' | 'comfortable' | 'spacious'
export type ReadingPanePosition = 'right' | 'bottom' | 'off'

interface PreferencesContextValue {
  listDensity: ListDensity
  readingPanePosition: ReadingPanePosition
  setListDensity: (density: ListDensity) => void
  setReadingPanePosition: (pos: ReadingPanePosition) => void
}

const PreferencesContext = createContext<PreferencesContextValue | null>(null)

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [listDensity, setListDensityState] = useState<ListDensity>(() => {
    const raw = getRawStorage('list_density')
    if (raw === 'compact' || raw === 'comfortable' || raw === 'spacious') return raw
    return 'comfortable'
  })

  const [readingPanePosition, setReadingPanePositionState] = useState<ReadingPanePosition>(() => {
    const raw = getRawStorage('reading_pane_pos')
    if (raw === 'right' || raw === 'bottom' || raw === 'off') return raw
    return 'right'
  })

  const setListDensity = (next: ListDensity) => {
    setListDensityState(next)
    setRawStorage('list_density', next)
  }

  const setReadingPanePosition = (next: ReadingPanePosition) => {
    setReadingPanePositionState(next)
    setRawStorage('reading_pane_pos', next)
  }

  return (
    <PreferencesContext.Provider
      value={{
        listDensity,
        readingPanePosition,
        setListDensity,
        setReadingPanePosition,
      }}
    >
      {children}
    </PreferencesContext.Provider>
  )
}

export function usePreferences(): PreferencesContextValue {
  const ctx = useContext(PreferencesContext)
  if (!ctx) throw new Error('usePreferences must be used inside PreferencesProvider')
  return ctx
}
