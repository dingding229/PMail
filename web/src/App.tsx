import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router'
import { Toaster } from 'sonner'
import { ThemeProvider } from '@/lib/ThemeProvider'
import { useTheme } from '@/lib/theme'
import { PreferencesProvider } from '@/lib/PreferencesProvider'
import { TooltipProvider } from '@/components/ui/tooltip'
import { OfflineBanner } from '@/components/common/OfflineBanner'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'
import { MaintenanceGate } from '@/components/common/MaintenanceGate'
import { router } from './router'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      gcTime: 10 * 60_000,
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
            <ErrorBoundary>
              <MaintenanceGate>
                <RouterProvider router={router} />
                <OfflineBanner />
                <ThemedToaster />
              </MaintenanceGate>
            </ErrorBoundary>
          </TooltipProvider>
        </PreferencesProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

export default App
