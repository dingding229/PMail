import { useState, type ReactNode } from 'react'
import { getRawStorage, setRawStorage } from './storage'
import {
  PreferencesContext,
  type ListDensity,
  type ReadingPanePosition,
} from './preferences'

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
