import { useState } from 'react'
import type { Delivery, DeliveryStatus } from '../types'

const statusColor: Record<DeliveryStatus, string> = {
  success: '#22c55e',
  failed: '#e55353',
  pending: '#9c9c9c',
  in_flight: '#056bff',
  held: '#6601e8',
}

interface Props {
  deliveries: Delivery[]
  apiKey: string
  onRedeliver: (id: string) => void
}

export function DeliveryLog({ deliveries, onRedeliver }: Props) {
  const [search, setSearch] = useState('')

  const filtered = search
    ? deliveries.filter((d) => {
        const q = search.toLowerCase()
        return (
          d.event_id.toLowerCase().includes(q) ||
          d.webhook_url.toLowerCase().includes(q) ||
          d.event_type.toLowerCase().includes(q) ||
          d.status.toLowerCase().includes(q)
        )
      })
    : deliveries

  return (
    <div>
      <input
        type="search"
        placeholder="Search by event ID, URL, type, or status…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={searchInput}
      />
      {filtered.length === 0 ? (
        <p style={{ color: '#9c9c9c', padding: '12px 0', fontSize: 13 }}>
          {deliveries.length === 0 ? 'No deliveries yet.' : 'No deliveries match your search.'}
        </p>
      ) : (
        <div style={{ overflowX: 'auto', overflowY: 'auto', maxHeight: 360, marginTop: 12 }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead style={{ position: 'sticky', top: 0, background: '#0d1635', zIndex: 1 }}>
              <tr style={{ color: '#9c9c9c', textAlign: 'left' }}>
                <th style={th}>Event ID</th>
                <th style={th}>Webhook</th>
                <th style={th}>Type</th>
                <th style={th}>#</th>
                <th style={th}>Status</th>
                <th style={th}>HTTP</th>
                <th style={th}>Latency</th>
                <th style={th}>Time</th>
                <th style={th}></th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((d) => (
                <tr key={d.id} style={{ borderTop: '1px solid #2c456d' }}>
                  <td style={td}>
                    <code style={{ fontSize: 10, color: '#9c9c9c' }}>{d.event_id.slice(0, 8)}…</code>
                  </td>
                  <td style={td}>
                    <code style={{ fontSize: 10, color: '#00b6ff' }}>{d.webhook_url}</code>
                  </td>
                  <td style={td}>{d.event_type}</td>
                  <td style={td}>{d.attempt}</td>
                  <td style={td}>
                    <span
                      style={{
                        background: statusColor[d.status] ?? '#6b7280',
                        color: '#fff',
                        borderRadius: 4,
                        padding: '2px 6px',
                        fontSize: 11,
                      }}
                    >
                      {d.status}
                    </span>
                  </td>
                  <td style={td}>{d.last_status_code ?? '—'}</td>
                  <td style={td}>{d.last_response_ms != null ? `${d.last_response_ms}ms` : '—'}</td>
                  <td style={td}>{new Date(d.updated_at).toLocaleTimeString()}</td>
                  <td style={td}>
                    {d.status === 'failed' && (
                      <button
                        style={{
                          fontSize: 11,
                          background: '#056bff',
                          color: '#fff',
                          border: 'none',
                          borderRadius: 4,
                          padding: '3px 8px',
                          cursor: 'pointer',
                        }}
                        onClick={() => onRedeliver(d.id)}
                      >
                        Re-deliver
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

const searchInput: React.CSSProperties = {
  width: '100%',
  background: '#182945',
  color: '#fafafa',
  border: '1px solid #2c456d',
  borderRadius: 6,
  padding: '7px 12px',
  fontSize: 13,
  outline: 'none',
}

const th: React.CSSProperties = { padding: '4px 8px', fontWeight: 500 }
const td: React.CSSProperties = { padding: '5px 8px', verticalAlign: 'middle' }
