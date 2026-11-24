import type { EmailChangeItem, EmailChangesResponse } from '@/api'

export const BACKOFF_SCHEDULE_MS = [2_000, 5_000, 10_000, 30_000] as const
export const HIDDEN_POLL_INTERVAL_MS = 60_000
export const DEFAULT_LONG_POLL_WAIT_SECONDS = 25

/**
 * Compute retry delay in milliseconds for consecutive errors (1-indexed).
 * Sequence: 2s -> 5s -> 10s -> 30s -> 30s...
 */
export function getBackoffDelayMs(consecutiveErrors: number): number {
  const idx = Math.max(0, Math.min(consecutiveErrors - 1, BACKOFF_SCHEDULE_MS.length - 1))
  return BACKOFF_SCHEDULE_MS[idx] ?? 30_000
}

export function isUnauthorizedError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const status = (err as { status?: unknown }).status
  return status === 401
}

export function isAbortError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  return (err as { name?: unknown }).name === 'AbortError'
}

export function formatChangeSender(item: EmailChangeItem): string {
  const name = item.from_name?.trim() || item.sender?.Name?.trim()
  const addr = item.from_address?.trim() || item.sender?.EmailAddress?.trim()
  return name || addr || ''
}

export function formatChangeSubject(item: EmailChangeItem): string {
  return (item.subject ?? item.title ?? '').trim()
}

export interface LiveUpdatesLoopOptions {
  fetchChanges: (params: {
    cursor: string
    wait: number
    signal: AbortSignal
  }) => Promise<EmailChangesResponse>
  isDocumentHidden: () => boolean
  onChanges: (res: EmailChangesResponse) => void
  onUnauthorized?: () => void
  scheduleTimer?: (ms: number, cb: () => void) => ReturnType<typeof setTimeout>
  clearTimer?: (id: ReturnType<typeof setTimeout>) => void
  hiddenIntervalMs?: number
  waitSeconds?: number
}

export interface LiveUpdatesLoopController {
  start: () => void
  stop: () => void
  handleVisibilityChange: () => void
  isStopped: () => boolean
}

/**
 * Pure, framework-agnostic long-polling loop with:
 * - cursor propagation (starts with empty string "")
 * - AbortController cancellation on stop()
 * - exponential-like backoff [2s, 5s, 10s, 30s, 30s...] reset on success
 * - immediate stop on HTTP 401
 * - reduced-frequency 60s wait=0 polling when document.hidden, immediate resume on visible
 */
export function createLiveUpdatesLoop(
  options: LiveUpdatesLoopOptions,
): LiveUpdatesLoopController {
  const scheduleTimer = options.scheduleTimer ?? ((ms, cb) => setTimeout(cb, ms))
  const clearTimer = options.clearTimer ?? ((id) => clearTimeout(id))
  const hiddenIntervalMs = options.hiddenIntervalMs ?? HIDDEN_POLL_INTERVAL_MS
  const waitSeconds = options.waitSeconds ?? DEFAULT_LONG_POLL_WAIT_SECONDS

  let stopped = false
  let running = false
  let cursor = ''
  let consecutiveErrors = 0
  let inFlight = false
  let abortController: AbortController | null = null
  let timerId: ReturnType<typeof setTimeout> | null = null
  let sleepingForHidden = false

  const cancelTimer = () => {
    if (timerId !== null) {
      clearTimer(timerId)
      timerId = null
    }
    sleepingForHidden = false
  }

  const scheduleNext = (delayMs: number, forHidden: boolean) => {
    if (stopped) return
    cancelTimer()
    sleepingForHidden = forHidden
    timerId = scheduleTimer(delayMs, () => {
      timerId = null
      sleepingForHidden = false
      void pollOnce()
    })
  }

  const pollOnce = async () => {
    if (stopped || inFlight) return
    inFlight = true
    const controller = new AbortController()
    abortController = controller
    const wait = options.isDocumentHidden() ? 0 : waitSeconds

    try {
      const res = await options.fetchChanges({
        cursor,
        wait,
        signal: controller.signal,
      })
      inFlight = false
      if (abortController === controller) abortController = null
      if (stopped) return

      consecutiveErrors = 0
      if (typeof res.cursor === 'string') {
        cursor = res.cursor
      }
      if (res.changed) {
        options.onChanges(res)
      }

      if (options.isDocumentHidden()) {
        scheduleNext(hiddenIntervalMs, true)
      } else {
        // Yield a microtask / 0ms tick so callers can observe state cleanly
        scheduleNext(0, false)
      }
    } catch (err) {
      inFlight = false
      if (abortController === controller) abortController = null
      if (stopped || isAbortError(err)) return

      if (isUnauthorizedError(err)) {
        stopped = true
        cancelTimer()
        options.onUnauthorized?.()
        return
      }

      consecutiveErrors += 1
      const backoff = getBackoffDelayMs(consecutiveErrors)
      scheduleNext(backoff, false)
    }
  }

  return {
    start() {
      if (running || stopped) return
      running = true
      void pollOnce()
    },
    stop() {
      if (stopped) return
      stopped = true
      cancelTimer()
      if (abortController) {
        abortController.abort()
        abortController = null
      }
    },
    handleVisibilityChange() {
      if (stopped || !running) return
      if (!options.isDocumentHidden() && sleepingForHidden) {
        cancelTimer()
        void pollOnce()
      }
    },
    isStopped() {
      return stopped
    },
  }
}

export interface ListUpdateDecision {
  shouldAutoRefreshList: boolean
  shouldShowPill: boolean
  shouldShowToast: boolean
}

/**
 * Decide how to display incoming changes based on list scroll position,
 * active multi-selection, and current folder.
 */
export function decideNewMailPresentation(params: {
  isScrolledDown: boolean
  hasMultiSelection: boolean
  isInbox: boolean
  newCount: number
}): ListUpdateDecision {
  const canAutoRefresh = !params.isScrolledDown && !params.hasMultiSelection
  const hasNewMail = params.newCount > 0
  return {
    shouldAutoRefreshList: canAutoRefresh,
    shouldShowPill: !canAutoRefresh && hasNewMail,
    shouldShowToast: hasNewMail && !(canAutoRefresh && params.isInbox),
  }
}
