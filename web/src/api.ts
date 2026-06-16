import type { Delivery, Event, VolumePoint, VolumeWindow, Webhook } from './types'

async function apiFetch<T>(path: string, apiKey: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    ...init,
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!resp.ok) {
    throw new Error(`${resp.status} ${resp.statusText}`)
  }
  return resp.json() as Promise<T>
}

export function fetchConfig(): Promise<{ api_key: string }> {
  return fetch('/config').then(r => r.json())
}

export function fetchWebhooks(apiKey: string): Promise<Webhook[]> {
  return apiFetch('/webhooks', apiKey)
}

export function fetchEvents(apiKey: string, limit = 50): Promise<Event[]> {
  return apiFetch(`/events?limit=${limit}`, apiKey)
}

export function fetchDeliveries(apiKey: string, limit = 100): Promise<Delivery[]> {
  return apiFetch(`/deliveries?limit=${limit}`, apiKey)
}

export function fetchVolume(apiKey: string, window: VolumeWindow): Promise<VolumePoint[]> {
  return apiFetch(`/events/volume?window=${window}`, apiKey)
}

export function redeliver(apiKey: string, deliveryId: string): Promise<Delivery> {
  return apiFetch(`/deliveries/${deliveryId}/redeliver`, apiKey, { method: 'POST' })
}

export function setCircuit(apiKey: string, webhookId: string, open: boolean): Promise<Webhook> {
  return apiFetch(`/webhooks/${webhookId}/circuit`, apiKey, {
    method: 'POST',
    body: JSON.stringify({ action: open ? 'open' : 'close' }),
  })
}
