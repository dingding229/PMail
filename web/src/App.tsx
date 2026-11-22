import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router'
import { Toaster } from 'sonner'
import { ThemeProvider, useTheme } from '@/lib/theme'
import { PreferencesProvider } from '@/lib/preferences'
import { TooltipProvider } from '@/components/ui/tooltip'
import { OfflineBanner } from '@/components/common/OfflineBanner'
import { router } from './router'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

function ThemedToaster() {
  const { resolved } = useTheme()
  return (
    <Toaster
      theme={resolved}
      position="bottom-right"
      richColors
      closeButton
      toastOptions={{
        duration: 4200,
      }}
    />
  )
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <PreferencesProvider>
          <TooltipProvider delayDuration={350}>
            <RouterProvider router={router} />
            <OfflineBanner />
            <ThemedToaster />
          </TooltipProvider>
        </PreferencesProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

export default App
