import { useEffect } from 'react'

export interface MailShortcutHandlers {
  onNext: () => void
  onPrev: () => void
  onOpen: () => void
  onArchive: () => void
  onDelete: () => void
  onRestore?: () => void
  onStar: () => void
  onReply: () => void
  onReplyAll: () => void
  onForward: () => void
  onCompose: () => void
  onSearch: () => void
  onEscape: () => void
  onHelp: () => void
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return true
  if (target.isContentEditable) return true
  if (target.closest('[contenteditable="true"]')) return true
  return false
}

export function useKeyboardShortcuts(handlers: MailShortcutHandlers, disabled = false) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey) return

      if (event.key === 'Escape') {
        handlers.onEscape()
        return
      }

      if (disabled || isEditableTarget(event.target)) return

      switch (event.key) {
        case 'j':
        case 'ArrowDown':
          event.preventDefault()
          handlers.onNext()
          break
        case 'k':
        case 'ArrowUp':
          event.preventDefault()
          handlers.onPrev()
          break
        case 'Enter':
        case 'o':
          event.preventDefault()
          handlers.onOpen()
          break
        case 'e':
          event.preventDefault()
          handlers.onArchive()
          break
        case '#':
          event.preventDefault()
          handlers.onDelete()
          break
        case 'z':
        case 'u':
          if (handlers.onRestore) {
            event.preventDefault()
            handlers.onRestore()
          }
          break
        case 's':
          event.preventDefault()
          handlers.onStar()
          break
        case 'r':
          event.preventDefault()
          handlers.onReply()
          break
        case 'a':
          event.preventDefault()
          handlers.onReplyAll()
          break
        case 'f':
          event.preventDefault()
          handlers.onForward()
          break
        case 'c':
          event.preventDefault()
          handlers.onCompose()
          break
        case '/':
          event.preventDefault()
          handlers.onSearch()
          break
        case '?':
          event.preventDefault()
          handlers.onHelp()
          break
        default:
          break
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [handlers, disabled])
}
