import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi, describe, it, expect } from 'vitest'
import { DeliveryLog } from './DeliveryLog'
import type { Delivery } from '../types'

const dl = (overrides: Partial<Delivery> = {}): Delivery => ({
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

describe('DeliveryLog', () => {
  it('renders event type, status, and attempt number', () => {
    render(<DeliveryLog deliveries={[dl()]} apiKey="k" onRedeliver={vi.fn()} />)
    expect(screen.getByText('order.created')).toBeInTheDocument()
    expect(screen.getByText('success')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('shows HTTP status code and latency', () => {
    render(<DeliveryLog deliveries={[dl()]} apiKey="k" onRedeliver={vi.fn()} />)
    expect(screen.getByText('200')).toBeInTheDocument()
    expect(screen.getByText('42ms')).toBeInTheDocument()
  })

  it('shows Re-deliver Now button only on failed deliveries', () => {
    render(
      <DeliveryLog
        deliveries={[dl({ status: 'failed' })]}
        apiKey="k"
        onRedeliver={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: /re-deliver/i })).toBeInTheDocument()
  })

  it('does not show Re-deliver Now button on success', () => {
    render(<DeliveryLog deliveries={[dl()]} apiKey="k" onRedeliver={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /re-deliver/i })).toBeNull()
  })

  it('calls onRedeliver with delivery id when button clicked', async () => {
    const onRedeliver = vi.fn()
    render(
      <DeliveryLog
        deliveries={[dl({ status: 'failed' })]}
        apiKey="k"
        onRedeliver={onRedeliver}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: /re-deliver/i }))
    expect(onRedeliver).toHaveBeenCalledWith('dl-1')
  })

  it('renders empty state when no deliveries', () => {
    render(<DeliveryLog deliveries={[]} apiKey="k" onRedeliver={vi.fn()} />)
    expect(screen.getByText(/no deliveries/i)).toBeInTheDocument()
  })
})
