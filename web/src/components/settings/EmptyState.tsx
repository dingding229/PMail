import type { ComponentType, ReactNode } from 'react'
import { cn } from '@/lib/cn'

export interface EmptyStateProps {
  icon: ComponentType<{ className?: string }>
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  compact?: boolean
  className?: string
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  compact = false,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center rounded-[var(--radius-md)] border border-dashed border-[var(--border-subtle)] bg-[var(--surface-canvas)]/60 px-6 text-center',
        compact ? 'py-8' : 'py-12',
        className,
      )}
    >
      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--surface-subtle)] text-[var(--ink-muted)]">
        <Icon className="h-5 w-5" />
      </div>
      <p className="mt-3 text-[13.5px] font-medium text-[var(--ink-primary)]">{title}</p>
      {description && (
        <p className="mt-1 max-w-md text-[12.5px] leading-relaxed text-[var(--ink-muted)]">
          {description}
        </p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
