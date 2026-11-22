import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/cn'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-[var(--radius-sm)] text-[13px] font-medium transition-colors duration-[var(--duration-fast)] ease-[var(--ease-standard)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent-ring)] disabled:pointer-events-none disabled:opacity-60 select-none shrink-0',
  {
    variants: {
      variant: {
        primary:
          'bg-[var(--accent)] text-white shadow-[var(--shadow-xs)] hover:bg-[var(--accent-hover)]',
        secondary:
          'bg-[var(--surface-subtle)] text-[var(--ink-primary)] border border-[var(--border-subtle)] hover:bg-[var(--surface-hover)] hover:border-[var(--border-strong)]',
        outline:
          'border border-[var(--border-subtle)] bg-[var(--surface-canvas)]/50 text-[var(--ink-primary)] hover:bg-[var(--surface-hover)] hover:border-[var(--border-strong)]',
        ghost:
          'bg-transparent text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
        danger:
          'bg-[var(--danger)] text-white hover:bg-[var(--danger-hover)]',
        dangerGhost:
          'bg-transparent text-[var(--danger)] hover:bg-[var(--danger-soft)]',
      },
      size: {
        xs: 'h-7 px-2 text-xs',
        sm: 'h-8 px-2.5 text-[13px]',
        md: 'h-9 px-3.5 text-[13.5px]',
        lg: 'h-10 px-4 text-sm',
        icon: 'h-8 w-8 p-0',
        iconSm: 'h-7 w-7 p-0',
      },
    },
    defaultVariants: {
      variant: 'primary',
      size: 'md',
    },
  },
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button'
    return (
      <Comp
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        {...props}
      />
    )
  },
)
Button.displayName = 'Button'
