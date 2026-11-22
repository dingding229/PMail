import { Check, Languages } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getAppLanguage, setAppLanguage, type SupportedLanguage } from '@/i18n'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

interface LanguageToggleProps {
  align?: 'start' | 'center' | 'end'
  onLanguageChange?: (lang: SupportedLanguage) => void
}

export function LanguageToggle({ align = 'end', onLanguageChange }: LanguageToggleProps) {
  const { t } = useTranslation()
  const current = getAppLanguage()

  const handleSelect = (lang: SupportedLanguage) => {
    setAppLanguage(lang)
    onLanguageChange?.(lang)
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('language.label')}
          title={t('language.label')}
        >
          <Languages className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align}>
        <DropdownMenuItem onClick={() => handleSelect('zh-CN')} className="justify-between">
          <span>{t('language.zhCN')}</span>
          {current === 'zh-CN' && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleSelect('en')} className="justify-between">
          <span>{t('language.en')}</span>
          {current === 'en' && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
