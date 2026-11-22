import * as React from 'react'
import { cn } from '@/lib/cn'

export type InputProps = React.InputHTMLAttributes<HTMLInputElement>

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = 'text', ...props }, ref) => (
    <input
      ref={ref}
      type={type}
      className={cn(
        'flex h-9 w-full rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-3 py-1.5 text-[13.5px] text-[var(--ink-primary)] placeholder:text-[var(--ink-subtle)] shadow-[var(--shadow-xs)] transition-colors duration-[var(--duration-fast)] focus-visible:outline-none focus-visible:border-[var(--accent)] focus-visible:ring-2 focus-visible:ring-[var(--accent-ring)] disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  ),
)
Input.displayName = 'Input'
