import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { EndpointHealth } from './EndpointHealth'
import type { Delivery, Webhook } from '../types'

const wh = (): Webhook => ({
  id: 'wh-1',
  url: 'https://example.com/hook',
  secret_hint: 'abc***',
  status: 'active',
  failure_streak: 2,
  circuit_threshold: 5,
  next_probe_at: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
})

const dl = (overrides: Partial<Delivery> = {}): Delivery => ({
  id: 'dl-1',
  event_id: 'ev-1',
  webhook_id: 'wh-1',
  parent_delivery_id: null,
  status: 'success',
  attempt: 1,
  next_attempt_at: null,
  last_status_code: 200,
  last_response_ms: 80,
  last_error: null,
  event_type: 'order.created',
  webhook_url: 'https://example.com/hook',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
})

describe('EndpointHealth', () => {
  it('renders webhook URL', () => {
    render(<EndpointHealth webhooks={[wh()]} deliveries={[dl()]} />)
    expect(screen.getByText('https://example.com/hook')).toBeInTheDocument()
  })

  it('shows 100% success rate for all-success deliveries', () => {
    render(<EndpointHealth webhooks={[wh()]} deliveries={[dl(), dl({ id: 'dl-2' })]} />)
    expect(screen.getByText('100%')).toBeInTheDocument()
  })

  it('shows 50% success rate for mixed deliveries', () => {
    render(
      <EndpointHealth
        webhooks={[wh()]}
        deliveries={[dl(), dl({ id: 'dl-2', status: 'failed' })]}
      />,
    )
    expect(screen.getByText('50%')).toBeInTheDocument()
  })

  it('shows failure streak', () => {
    render(<EndpointHealth webhooks={[wh()]} deliveries={[dl()]} />)
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('renders empty state with no webhooks', () => {
    render(<EndpointHealth webhooks={[]} deliveries={[]} />)
    expect(screen.getByText(/no webhooks/i)).toBeInTheDocument()
  })
})
