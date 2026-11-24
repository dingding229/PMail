import type { ReactNode } from 'react'
import { cn } from '@/lib/cn'
import { Label } from '@/components/ui/label'

export interface FormRowProps {
  label?: ReactNode
  htmlFor?: string
  description?: ReactNode
  descriptionId?: string
  error?: ReactNode
  layout?: 'stacked' | 'horizontal'
  surface?: boolean
  className?: string
  children: ReactNode
}

export function FormRow({
  label,
  htmlFor,
  description,
  descriptionId,
  error,
  layout = 'stacked',
  surface = false,
  className,
  children,
}: FormRowProps) {
  if (layout === 'horizontal') {
    return (
      <div
        className={cn(
          'flex items-center justify-between gap-4',
          surface &&
            'rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3',
          className,
        )}
      >
        <div className="min-w-0">
          {label && (
            <Label
              htmlFor={htmlFor}
              className="text-[13.5px] font-medium text-[var(--ink-primary)]"
            >
              {label}
            </Label>
          )}
          {description && (
            <p
              id={descriptionId}
              className="mt-0.5 text-[12px] leading-relaxed text-[var(--ink-muted)]"
            >
              {description}
            </p>
          )}
          {error && <p className="mt-1 text-[12px] text-[var(--danger)]">{error}</p>}
        </div>
        <div className="shrink-0">{children}</div>
      </div>
    )
  }

  return (
    <div
      className={cn(
        'space-y-1.5',
        surface &&
          'rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4',
        className,
      )}
    >
      {label && (
        <Label htmlFor={htmlFor} className="block">
          {label}
        </Label>
      )}
      {children}
      {description && (
        <p
          id={descriptionId}
          className="text-[12px] leading-relaxed text-[var(--ink-muted)]"
        >
          {description}
        </p>
      )}
      {error && <p className="text-[12px] text-[var(--danger)]">{error}</p>}
    </div>
  )
}

export function SectionFooter({
  children,
  align = 'end',
  className,
}: {
  children: ReactNode
  align?: 'end' | 'between' | 'start'
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-2 border-t border-[var(--border-hairline)] pt-4',
        align === 'between'
          ? 'justify-between'
          : align === 'start'
            ? 'justify-start'
            : 'justify-end',
        className,
      )}
    >
      {children}
    </div>
  )
}
