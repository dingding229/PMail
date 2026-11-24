import { useEffect, useRef } from 'react'
import { mailApi, type EmailChangesResponse } from '@/api'
import {
  createLiveUpdatesLoop,
  formatChangeSender,
  formatChangeSubject,
  type LiveUpdatesLoopController,
} from './liveUpdatesCore'

const CHANNEL_NAME = 'pmail_live_updates_v1'
const HEARTBEAT_INTERVAL_MS = 4_000
const LEADER_TIMEOUT_MS = 9_000
const ELECTION_WAIT_MS = 350
const MAX_NOTIFIED_IDS = 200

type BroadcastPayload =
  | { type: 'heartbeat'; tabId: string; ts: number }
  | { type: 'claim'; tabId: string; ts: number }
  | { type: 'resign'; tabId: string }
  | { type: 'changes'; tabId: string; data: EmailChangesResponse }

export interface UseMailLiveUpdatesOptions {
  enabled: boolean
  desktopNotifications: boolean
  noSubjectLabel: string
  onChanges: (res: EmailChangesResponse) => void
  onOpenEmail: (id: number) => void
}

/**
 * Mount once inside MailWorkspace.
 * Uses BroadcastChannel leader election so only one tab holds an active
 * long-poll connection, while follower tabs receive changes over the channel.
 * Falls back to independent polling when BroadcastChannel is unsupported.
 */
export function useMailLiveUpdates({
  enabled,
  desktopNotifications,
  noSubjectLabel,
  onChanges,
  onOpenEmail,
}: UseMailLiveUpdatesOptions) {
  const onChangesRef = useRef(onChanges)
  const onOpenEmailRef = useRef(onOpenEmail)
  const desktopNotificationsRef = useRef(desktopNotifications)
  const noSubjectLabelRef = useRef(noSubjectLabel)
  const notifiedIdsRef = useRef<Set<number>>(new Set())

  useEffect(() => {
    onChangesRef.current = onChanges
  }, [onChanges])

  useEffect(() => {
    onOpenEmailRef.current = onOpenEmail
  }, [onOpenEmail])

  useEffect(() => {
    desktopNotificationsRef.current = desktopNotifications
  }, [desktopNotifications])

  useEffect(() => {
    noSubjectLabelRef.current = noSubjectLabel
  }, [noSubjectLabel])

  useEffect(() => {
    if (!enabled) return

    const tabId = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
    let loop: LiveUpdatesLoopController | null = null
    let isLeader = false
    let disposed = false
    let channel: BroadcastChannel | null = null
    let heartbeatTimer: ReturnType<typeof setInterval> | null = null
    let watchdogTimer: ReturnType<typeof setInterval> | null = null
    let electionTimer: ReturnType<typeof setTimeout> | null = null
    let lastLeaderSeenAt = 0

    const fireDesktopNotifications = (res: EmailChangesResponse) => {
      if (!desktopNotificationsRef.current) return
      if (typeof document === 'undefined' || !document.hidden) return
      if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return

      const notified = notifiedIdsRef.current
      for (const item of res.new_items ?? []) {
        if (!item?.id || notified.has(item.id)) continue
        notified.add(item.id)
        if (notified.size > MAX_NOTIFIED_IDS) {
          const oldest = notified.values().next().value
          if (oldest !== undefined) notified.delete(oldest)
        }
        const sender = formatChangeSender(item) || 'PMail'
        const subject = formatChangeSubject(item) || noSubjectLabelRef.current
        try {
          const notification = new Notification(sender, {
            body: subject,
            tag: `pmail-email-${item.id}`,
          })
          notification.onclick = () => {
            window.focus()
            onOpenEmailRef.current(item.id)
            notification.close()
          }
        } catch {
          // Ignore browser Notification constructor errors
        }
      }
    }

    const startLeaderLoop = () => {
      if (disposed || isLeader) return
      isLeader = true
      if (channel) {
        channel.postMessage({ type: 'claim', tabId, ts: Date.now() } satisfies BroadcastPayload)
        if (heartbeatTimer) clearInterval(heartbeatTimer)
        heartbeatTimer = setInterval(() => {
          channel?.postMessage({
            type: 'heartbeat',
            tabId,
            ts: Date.now(),
          } satisfies BroadcastPayload)
        }, HEARTBEAT_INTERVAL_MS)
      }

      loop = createLiveUpdatesLoop({
        fetchChanges: ({ cursor, wait, signal }) =>
          mailApi.changes({ cursor, wait, signal }),
        isDocumentHidden: () => (typeof document !== 'undefined' ? document.hidden : false),
        onChanges: (res) => {
          if (channel) {
            channel.postMessage({
              type: 'changes',
              tabId,
              data: res,
            } satisfies BroadcastPayload)
          }
          fireDesktopNotifications(res)
          onChangesRef.current(res)
        },
      })
      loop.start()
    }

    const stopLeaderLoop = () => {
      if (!isLeader) return
      isLeader = false
      if (heartbeatTimer) {
        clearInterval(heartbeatTimer)
        heartbeatTimer = null
      }
      if (loop) {
        loop.stop()
        loop = null
      }
    }

    const handleVisibility = () => {
      loop?.handleVisibilityChange()
    }

    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibility)
    }

    const handleBeforeUnload = () => {
      if (isLeader && channel) {
        channel.postMessage({ type: 'resign', tabId } satisfies BroadcastPayload)
      }
    }

    if (typeof window !== 'undefined') {
      window.addEventListener('beforeunload', handleBeforeUnload)
    }

    if (typeof BroadcastChannel !== 'undefined') {
      try {
        channel = new BroadcastChannel(CHANNEL_NAME)
        channel.onmessage = (event: MessageEvent<BroadcastPayload>) => {
          const msg = event.data
          if (!msg || msg.tabId === tabId) return

          if (msg.type === 'heartbeat' || msg.type === 'claim') {
            lastLeaderSeenAt = Date.now()
            if (isLeader && msg.tabId < tabId) {
              // Deterministic tie-breaker: lower tabId wins leadership
              stopLeaderLoop()
            }
          } else if (msg.type === 'resign') {
            lastLeaderSeenAt = 0
            if (!isLeader) {
              if (electionTimer) clearTimeout(electionTimer)
              electionTimer = setTimeout(() => {
                if (!disposed && Date.now() - lastLeaderSeenAt > ELECTION_WAIT_MS) {
                  startLeaderLoop()
                }
              }, Math.floor(Math.random() * 150) + 50)
            }
          } else if (msg.type === 'changes') {
            lastLeaderSeenAt = Date.now()
            if (!isLeader) {
              onChangesRef.current(msg.data)
            }
          }
        }

        // Wait briefly to see if another tab is already leader
        electionTimer = setTimeout(() => {
          if (!disposed && lastLeaderSeenAt === 0) {
            startLeaderLoop()
          }
        }, ELECTION_WAIT_MS)

        // Watchdog promotes a follower if the leader tab crashes silently
        watchdogTimer = setInterval(() => {
          if (!disposed && !isLeader && Date.now() - lastLeaderSeenAt > LEADER_TIMEOUT_MS) {
            startLeaderLoop()
          }
        }, HEARTBEAT_INTERVAL_MS)
      } catch {
        // Fallback if BroadcastChannel constructor is blocked
        startLeaderLoop()
      }
    } else {
      startLeaderLoop()
    }

    return () => {
      disposed = true
      if (electionTimer) clearTimeout(electionTimer)
      if (watchdogTimer) clearInterval(watchdogTimer)
      if (isLeader && channel) {
        try {
          channel.postMessage({ type: 'resign', tabId } satisfies BroadcastPayload)
        } catch {
          // ignore
        }
      }
      stopLeaderLoop()
      if (channel) {
        channel.close()
        channel = null
      }
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', handleVisibility)
      }
      if (typeof window !== 'undefined') {
        window.removeEventListener('beforeunload', handleBeforeUnload)
      }
    }
  }, [enabled])
}
