import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TooltipProvider } from '@/components/ui/tooltip'
import App from './App'
import './index.css'
const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 2000, refetchInterval: 5000 } } })
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode><QueryClientProvider client={queryClient}><TooltipProvider delayDuration={200}><App /></TooltipProvider></QueryClientProvider></React.StrictMode>,
)
