import { beforeEach, describe, expect, it } from 'vitest'
import { useDashboardStore } from './store'
import type { Delivery, Event, Webhook } from './types'

const webhookFixture = (overrides: Partial<Webhook> = {}): Webhook => ({
  id: 'wh-1',
  url: 'https://example.com/hook',
  secret_hint: 'abc***',
  status: 'active',
  failure_streak: 0,
  circuit_threshold: 5,
  next_probe_at: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
})

const eventFixture = (overrides: Partial<Event> = {}): Event => ({
  id: 'ev-1',
  type: 'order.created',
  source: 'https://sim.local',
  time: '2026-01-01T00:00:00Z',
  data: {},
  received_at: '2026-01-01T00:00:00Z',
  ...overrides,
})

const deliveryFixture = (overrides: Partial<Delivery> = {}): Delivery => ({
  id: 'dl-1',
  event_id: 'ev-1',
  webhook_id: 'wh-1',
  parent_delivery_id: null,
  status: 'success',
  attempt: 1,
  next_attempt_at: null,
  last_status_code: 200,
  last_response_ms: 42,
  last_error: null,
  event_type: 'order.created',
  webhook_url: 'https://example.com/hook',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
})

beforeEach(() => {
  useDashboardStore.setState({
    apiKey: '',
    webhooks: [],
    events: [],
    deliveries: [],
    volumeData: [],
    volumeWindow: '30m',
  })
})

describe('applySSEEvent', () => {
  it('webhook_updated — adds new webhook when not in list', () => {
    useDashboardStore.getState().applySSEEvent('webhook_updated', webhookFixture())
    expect(useDashboardStore.getState().webhooks).toHaveLength(1)
    expect(useDashboardStore.getState().webhooks[0].id).toBe('wh-1')
  })

  it('webhook_updated — updates existing webhook in place', () => {
    useDashboardStore.setState({ webhooks: [webhookFixture()] })
    useDashboardStore
      .getState()
      .applySSEEvent('webhook_updated', webhookFixture({ status: 'circuit_open' }))
    expect(useDashboardStore.getState().webhooks).toHaveLength(1)
    expect(useDashboardStore.getState().webhooks[0].status).toBe('circuit_open')
  })

  it('event_ingested — prepends event and caps at 50', () => {
    const existing = Array.from({ length: 50 }, (_, i) =>
      eventFixture({ id: `ev-old-${i}` }),
    )
    useDashboardStore.setState({ events: existing })
    useDashboardStore.getState().applySSEEvent('event_ingested', eventFixture({ id: 'ev-new' }))
    const { events } = useDashboardStore.getState()
    expect(events).toHaveLength(50)
    expect(events[0].id).toBe('ev-new')
  })

  it('event_ingested — increments existing volume type count', () => {
    useDashboardStore.setState({ volumeData: [{ type: 'order.created', count: 3 }] })
    useDashboardStore.getState().applySSEEvent('event_ingested', eventFixture())
    expect(useDashboardStore.getState().volumeData[0].count).toBe(4)
  })

  it('event_ingested — adds new type to volumeData if not present', () => {
    useDashboardStore.getState().applySSEEvent('event_ingested', eventFixture({ type: 'new.type' }))
    const { volumeData } = useDashboardStore.getState()
    expect(volumeData.find(v => v.type === 'new.type')?.count).toBe(1)
  })

  it('delivery_updated — adds new delivery when not in list', () => {
    useDashboardStore.getState().applySSEEvent('delivery_updated', deliveryFixture())
    expect(useDashboardStore.getState().deliveries).toHaveLength(1)
  })

  it('delivery_updated — updates existing delivery', () => {
    useDashboardStore.setState({ deliveries: [deliveryFixture()] })
    useDashboardStore
      .getState()
      .applySSEEvent('delivery_updated', deliveryFixture({ status: 'failed' }))
    expect(useDashboardStore.getState().deliveries[0].status).toBe('failed')
  })
})
