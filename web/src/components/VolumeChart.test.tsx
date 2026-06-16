import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi, describe, it, expect } from 'vitest'
import { VolumeChart } from './VolumeChart'
import type { VolumePoint } from '../types'

// recharts uses SVG — jsdom doesn't support ResizeObserver
global.ResizeObserver = class {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const data: VolumePoint[] = [
  { type: 'order.created', count: 10 },
  { type: 'payment.failed', count: 3 },
]

describe('VolumeChart', () => {
  it('renders event type labels from data', () => {
    render(<VolumeChart data={data} window="30m" onWindowChange={vi.fn()} />)
    expect(screen.getByText('order.created')).toBeInTheDocument()
    expect(screen.getByText('payment.failed')).toBeInTheDocument()
  })

  it('renders window selector with current value selected', () => {
    render(<VolumeChart data={data} window="30m" onWindowChange={vi.fn()} />)
    const select = screen.getByRole('combobox')
    expect((select as HTMLSelectElement).value).toBe('30m')
  })

  it('calls onWindowChange when dropdown changes', async () => {
    const onWindowChange = vi.fn()
    render(<VolumeChart data={data} window="30m" onWindowChange={onWindowChange} />)
    await userEvent.selectOptions(screen.getByRole('combobox'), '1h')
    expect(onWindowChange).toHaveBeenCalledWith('1h')
  })

  it('renders empty state with no data', () => {
    render(<VolumeChart data={[]} window="30m" onWindowChange={vi.fn()} />)
    expect(screen.getByText(/no data/i)).toBeInTheDocument()
  })
})
