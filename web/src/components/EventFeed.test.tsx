import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { EventFeed } from './EventFeed'
import type { Event } from '../types'

const ev = (overrides: Partial<Event> = {}): Event => ({
  id: 'ev-1',
  type: 'order.created',
  source: 'https://sim.local',
  time: '2026-01-01T12:00:00Z',
  data: { foo: 'bar' },
  received_at: '2026-01-01T12:00:00Z',
  ...overrides,
})

describe('EventFeed', () => {
  it('renders event type and source', () => {
    render(<EventFeed events={[ev()]} />)
    expect(screen.getByText('order.created')).toBeInTheDocument()
    expect(screen.getByText('https://sim.local')).toBeInTheDocument()
  })

  it('shows truncated data preview', () => {
    render(<EventFeed events={[ev()]} />)
    expect(screen.getByText(/foo/)).toBeInTheDocument()
  })

  it('renders empty state when no events', () => {
    render(<EventFeed events={[]} />)
    expect(screen.getByText(/no events/i)).toBeInTheDocument()
  })
})
