import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useDashboardStore } from '../store'

export function useSSE() {
  const apiKey = useDashboardStore((s) => s.apiKey)
  const applySSEEvent = useDashboardStore((s) => s.applySSEEvent)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!apiKey) return

    const es = new EventSource(`/stream?key=${encodeURIComponent(apiKey)}`)

    es.addEventListener('webhook_updated', (e: MessageEvent) => {
      applySSEEvent('webhook_updated', JSON.parse(e.data))
    })
    es.addEventListener('event_ingested', (e: MessageEvent) => {
      applySSEEvent('event_ingested', JSON.parse(e.data))
    })
    es.addEventListener('delivery_updated', (e: MessageEvent) => {
      applySSEEvent('delivery_updated', JSON.parse(e.data))
    })
    es.onerror = () => {
      // On disconnect: invalidate all queries so they re-hydrate when stream reconnects.
      queryClient.invalidateQueries()
      es.close()
    }

    return () => es.close()
  }, [apiKey, applySSEEvent, queryClient])
}
