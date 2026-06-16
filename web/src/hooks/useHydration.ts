import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import {
  fetchConfig,
  fetchDeliveries,
  fetchEvents,
  fetchVolume,
  fetchWebhooks,
} from '../api'
import { useDashboardStore } from '../store'

export function useHydration() {
  const apiKey = useDashboardStore((s) => s.apiKey)
  const volumeWindow = useDashboardStore((s) => s.volumeWindow)
  const setApiKey = useDashboardStore((s) => s.setApiKey)
  const setWebhooks = useDashboardStore((s) => s.setWebhooks)
  const setEvents = useDashboardStore((s) => s.setEvents)
  const setDeliveries = useDashboardStore((s) => s.setDeliveries)
  const setVolumeData = useDashboardStore((s) => s.setVolumeData)

  const { data: config } = useQuery({
    queryKey: ['config'],
    queryFn: fetchConfig,
    staleTime: Infinity,
  })

  useEffect(() => {
    if (config?.api_key && config.api_key !== apiKey) {
      setApiKey(config.api_key)
    }
  }, [config, apiKey, setApiKey])

  const { data: webhooks } = useQuery({
    queryKey: ['webhooks'],
    queryFn: () => fetchWebhooks(apiKey),
    enabled: !!apiKey,
  })

  const { data: events } = useQuery({
    queryKey: ['events'],
    queryFn: () => fetchEvents(apiKey),
    enabled: !!apiKey,
  })

  const { data: deliveries } = useQuery({
    queryKey: ['deliveries'],
    queryFn: () => fetchDeliveries(apiKey),
    enabled: !!apiKey,
  })

  const { data: volumeData } = useQuery({
    queryKey: ['volume', volumeWindow],
    queryFn: () => fetchVolume(apiKey, volumeWindow),
    enabled: !!apiKey,
  })

  useEffect(() => { if (webhooks) setWebhooks(webhooks) }, [webhooks, setWebhooks])
  useEffect(() => { if (events) setEvents(events) }, [events, setEvents])
  useEffect(() => { if (deliveries) setDeliveries(deliveries) }, [deliveries, setDeliveries])
  useEffect(() => { if (volumeData) setVolumeData(volumeData) }, [volumeData, setVolumeData])
}
