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

  const [autoRefresh, setAutoRefreshState] = useState<boolean>(() => {
    const raw = getRawStorage('auto_refresh')
    if (raw === '0' || raw === 'false') return false
    return true
  })

  const [desktopNotifications, setDesktopNotificationsState] = useState<boolean>(() => {
    const raw = getRawStorage('desktop_notifications')
    return raw === '1' || raw === 'true'
  })

  const [showBrandAvatars, setShowBrandAvatarsState] = useState<boolean>(() => {
    const raw = getRawStorage('show_brand_avatars')
    if (raw === '0' || raw === 'false') return false
    return true
  })

  const setListDensity = (next: ListDensity) => {
    setListDensityState(next)
    setRawStorage('list_density', next)
  }

  const setReadingPanePosition = (next: ReadingPanePosition) => {
    setReadingPanePositionState(next)
    setRawStorage('reading_pane_pos', next)
  }

  const setAutoRefresh = (next: boolean) => {
    setAutoRefreshState(next)
    setRawStorage('auto_refresh', next ? '1' : '0')
  }

  const setDesktopNotifications = (next: boolean) => {
    setDesktopNotificationsState(next)
    setRawStorage('desktop_notifications', next ? '1' : '0')
  }

  const setShowBrandAvatars = (next: boolean) => {
    setShowBrandAvatarsState(next)
    setRawStorage('show_brand_avatars', next ? '1' : '0')
  }

  return (
    <PreferencesContext.Provider
      value={{
        listDensity,
        readingPanePosition,
        autoRefresh,
        desktopNotifications,
        showBrandAvatars,
        setListDensity,
        setReadingPanePosition,
        setAutoRefresh,
        setDesktopNotifications,
        setShowBrandAvatars,
      }}
    >
      {children}
    </PreferencesContext.Provider>
  )
}
