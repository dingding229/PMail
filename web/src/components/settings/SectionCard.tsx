import type { ComponentPropsWithoutRef, ReactNode } from 'react'
import { AlertCircle, RefreshCw } from 'lucide-react'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

export interface SectionCardProps extends ComponentPropsWithoutRef<'section'> {
  children: ReactNode
}

export function SectionCard({ className, children, ...props }: SectionCardProps) {
  return (
    <section
      className={cn(
        'space-y-5 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)]',
        className,
      )}
      {...props}
    >
      {children}
    </section>
  )
}

export function SectionLoadingState({
  rows = 3,
  rowHeight = 'h-16',
}: {
  rows?: number
  rowHeight?: string
}) {
  return (
    <div className="space-y-3" aria-busy="true">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton
          key={i}
          className={cn(rowHeight, 'w-full rounded-[var(--radius-md)]')}
        />
      ))}
    </div>
  )
}

export function SectionErrorState({
  message,
  retryLabel,
  onRetry,
}: {
  message: string
  retryLabel?: string
  onRetry?: () => void
}) {
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--danger)]/35 bg-[var(--danger-soft)] px-4 py-3 text-[13px] text-[var(--danger)]"
    >
      <div className="flex min-w-0 items-start gap-2.5">
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
        <p className="min-w-0 break-words">{message}</p>
      </div>
      {onRetry && retryLabel && (
        <Button type="button" variant="outline" size="xs" onClick={onRetry}>
          <RefreshCw className="h-3.5 w-3.5" />
          {retryLabel}
        </Button>
      )}
    </div>
  )
}
