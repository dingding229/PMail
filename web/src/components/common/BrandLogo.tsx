import { Mail } from 'lucide-react'
import { cn } from '@/lib/cn'

interface BrandLogoProps {
  compact?: boolean
  className?: string
}

export function BrandLogo({ compact = false, className }: BrandLogoProps) {
  return (
    <div className={cn('inline-flex items-center gap-2.5 select-none', className)}>
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[10px] bg-[var(--accent)] text-white shadow-[var(--shadow-xs)]">
        <Mail className="h-4 w-4 stroke-[2.2]" />
      </div>
      {!compact && (
        <div className="flex items-baseline gap-1.5 min-w-0">
          <span className="text-[15px] font-semibold tracking-tight text-[var(--ink-primary)] truncate">
            PMail
          </span>
          <span className="rounded-[5px] bg-[var(--accent-soft)] px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-[var(--accent)]">
            PRO
          </span>
        </div>
      )}
    </div>
  )
}
