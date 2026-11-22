import { cn } from '@/lib/cn'
import { getAvatarPalette, getInitials } from '@/lib/format'

interface SenderAvatarProps {
  name?: string
  email?: string
  size?: 'xs' | 'sm' | 'md' | 'lg'
  className?: string
}

const SIZE_CLASSES = {
  xs: 'h-6 w-6 text-[11px]',
  sm: 'h-8 w-8 text-[12px]',
  md: 'h-9 w-9 text-[13px]',
  lg: 'h-10 w-10 text-[14px]',
} as const

export function SenderAvatar({ name, email, size = 'md', className }: SenderAvatarProps) {
  const palette = getAvatarPalette(email || name || 'pmail')
  const initials = getInitials(name, email)
  return (
    <div
      aria-hidden="true"
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-full font-semibold select-none',
        SIZE_CLASSES[size],
        palette.bg,
        palette.text,
        className,
      )}
    >
      {initials}
    </div>
  )
}
