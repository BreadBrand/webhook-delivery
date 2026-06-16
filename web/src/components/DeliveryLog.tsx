import type { Delivery, DeliveryStatus } from '../types'

const statusColor: Record<DeliveryStatus, string> = {
  success: '#22c55e',
  failed: '#ef4444',
  pending: '#94a3b8',
  in_flight: '#38bdf8',
  held: '#f59e0b',
}

interface Props {
  deliveries: Delivery[]
  apiKey: string
  onRedeliver: (id: string) => void
}

export function DeliveryLog({ deliveries, onRedeliver }: Props) {
  if (deliveries.length === 0) {
    return <p style={{ color: '#94a3b8' }}>No deliveries yet.</p>
  }

  return (
    <div style={{ overflowX: 'auto' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
        <thead>
          <tr style={{ color: '#94a3b8', textAlign: 'left' }}>
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
          {deliveries.map((d) => (
            <tr key={d.id} style={{ borderTop: '1px solid #1e293b' }}>
              <td style={td}>
                <code style={{ fontSize: 10 }}>{d.event_id.slice(0, 8)}…</code>
              </td>
              <td style={td}>
                <code style={{ fontSize: 10 }}>{d.webhook_url}</code>
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
                  <button style={{ fontSize: 11 }} onClick={() => onRedeliver(d.id)}>
                    Re-deliver Now
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const th: React.CSSProperties = { padding: '4px 8px', fontWeight: 500 }
const td: React.CSSProperties = { padding: '5px 8px', verticalAlign: 'middle' }
