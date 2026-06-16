import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi, describe, it, expect } from 'vitest'
import { WebhookRegistry } from './WebhookRegistry'
import type { Webhook } from '../types'

const wh = (overrides: Partial<Webhook> = {}): Webhook => ({
  id: 'wh-1',
  url: 'https://example.com/hook',
  secret_hint: 'abc***xyz',
  status: 'active',
  failure_streak: 0,
  circuit_threshold: 5,
  next_probe_at: null,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
})

describe('WebhookRegistry', () => {
  it('renders webhook URL and status badge', () => {
    render(<WebhookRegistry webhooks={[wh()]} apiKey="k" onCircuitToggle={vi.fn()} />)
    expect(screen.getByText('https://example.com/hook')).toBeInTheDocument()
    expect(screen.getByText('active')).toBeInTheDocument()
  })

  it('shows masked secret hint', () => {
    render(<WebhookRegistry webhooks={[wh()]} apiKey="k" onCircuitToggle={vi.fn()} />)
    expect(screen.getByText('abc***xyz')).toBeInTheDocument()
  })

  it('shows Open Circuit button for active webhook', () => {
    render(<WebhookRegistry webhooks={[wh()]} apiKey="k" onCircuitToggle={vi.fn()} />)
    expect(screen.getByRole('button', { name: /open circuit/i })).toBeInTheDocument()
  })

  it('calls onCircuitToggle with correct args when Open Circuit clicked', async () => {
    const onToggle = vi.fn()
    render(<WebhookRegistry webhooks={[wh()]} apiKey="k" onCircuitToggle={onToggle} />)
    await userEvent.click(screen.getByRole('button', { name: /open circuit/i }))
    expect(onToggle).toHaveBeenCalledWith('wh-1', true)
  })

  it('shows Close Circuit button when circuit is open', () => {
    render(
      <WebhookRegistry
        webhooks={[wh({ status: 'circuit_open' })]}
        apiKey="k"
        onCircuitToggle={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: /close circuit/i })).toBeInTheDocument()
  })

  it('renders empty state when no webhooks', () => {
    render(<WebhookRegistry webhooks={[]} apiKey="k" onCircuitToggle={vi.fn()} />)
    expect(screen.getByText(/no webhooks/i)).toBeInTheDocument()
  })
})
