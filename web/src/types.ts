export type WebhookStatus = 'active' | 'degraded' | 'circuit_open' | 'deleted'
export type DeliveryStatus = 'pending' | 'in_flight' | 'success' | 'failed' | 'held'
export type VolumeWindow = '5m' | '30m' | '1h' | '24h'

export interface Webhook {
  id: string
  url: string
  secret_hint: string
  status: WebhookStatus
  failure_streak: number
  circuit_threshold: number
  next_probe_at: string | null
  created_at: string
  updated_at: string
}

export interface Event {
  id: string
  type: string
  source: string
  time: string
  data: unknown
  received_at: string
}

export interface Delivery {
  id: string
  event_id: string
  webhook_id: string
  parent_delivery_id: string | null
  status: DeliveryStatus
  attempt: number
  next_attempt_at: string | null
  last_status_code: number | null
  last_response_ms: number | null
  last_error: string | null
  event_type: string
  webhook_url: string
  created_at: string
  updated_at: string
}

export interface VolumePoint {
  type: string
  count: number
}
