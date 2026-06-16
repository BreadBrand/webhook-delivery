import { create } from 'zustand'
import type { Delivery, Event, VolumePoint, VolumeWindow, Webhook } from './types'

interface DashboardState {
  apiKey: string
  webhooks: Webhook[]
  events: Event[]
  deliveries: Delivery[]
  volumeData: VolumePoint[]
  volumeWindow: VolumeWindow
  setApiKey: (key: string) => void
  setWebhooks: (webhooks: Webhook[]) => void
  setEvents: (events: Event[]) => void
  setDeliveries: (deliveries: Delivery[]) => void
  setVolumeData: (data: VolumePoint[]) => void
  setVolumeWindow: (window: VolumeWindow) => void
  applySSEEvent: (type: string, data: unknown) => void
}

export const useDashboardStore = create<DashboardState>((set) => ({
  apiKey: '',
  webhooks: [],
  events: [],
  deliveries: [],
  volumeData: [],
  volumeWindow: '30m',

  setApiKey: (key) => set({ apiKey: key }),
  setWebhooks: (webhooks) => set({ webhooks }),
  setEvents: (events) => set({ events }),
  setDeliveries: (deliveries) => set({ deliveries }),
  setVolumeData: (volumeData) => set({ volumeData }),
  setVolumeWindow: (volumeWindow) => set({ volumeWindow }),

  applySSEEvent: (type, data) =>
    set((state) => {
      switch (type) {
        case 'webhook_updated': {
          const wh = data as Webhook
          const idx = state.webhooks.findIndex((w) => w.id === wh.id)
          const webhooks =
            idx >= 0
              ? state.webhooks.map((w, i) => (i === idx ? wh : w))
              : [wh, ...state.webhooks]
          return { webhooks }
        }
        case 'event_ingested': {
          const ev = data as Event
          const events = [ev, ...state.events].slice(0, 50)
          const existingIdx = state.volumeData.findIndex((v) => v.type === ev.type)
          const volumeData =
            existingIdx >= 0
              ? state.volumeData.map((v, i) =>
                  i === existingIdx ? { ...v, count: v.count + 1 } : v,
                )
              : [...state.volumeData, { type: ev.type, count: 1 }]
          return { events, volumeData }
        }
        case 'delivery_updated': {
          const d = data as Delivery
          const idx = state.deliveries.findIndex((x) => x.id === d.id)
          const deliveries =
            idx >= 0
              ? state.deliveries.map((x, i) => (i === idx ? d : x))
              : [d, ...state.deliveries]
          return { deliveries }
        }
        default:
          return state
      }
    }),
}))
