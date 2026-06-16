import type { Webhook } from '../types'

const statusColor: Record<string, string> = {
  active: '#22c55e',
  degraded: '#f59e0b',
  circuit_open: '#e55353',
  deleted: '#6b7280',
}

interface Props {
  webhooks: Webhook[]
  apiKey: string
  onCircuitToggle: (id: string, open: boolean) => void
}

export function WebhookRegistry({ webhooks, onCircuitToggle }: Props) {
  if (webhooks.length === 0) {
    return <p style={{ color: '#9c9c9c', fontSize: 13 }}>No webhooks registered yet.</p>
  }

  return (
    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
      <thead>
        <tr style={{ color: '#9c9c9c', textAlign: 'left' }}>
          <th style={th}>URL</th>
          <th style={th}>Status</th>
          <th style={th}>Secret hint</th>
          <th style={th}>Failures</th>
          <th style={th}>Circuit</th>
        </tr>
      </thead>
      <tbody>
        {webhooks.map((wh) => (
          <tr key={wh.id} style={{ borderTop: '1px solid #2c456d' }}>
            <td style={td}>
              <code style={{ fontSize: 12, color: '#00b6ff' }}>{wh.url}</code>
            </td>
            <td style={td}>
              <span
                style={{
                  background: statusColor[wh.status] ?? '#6b7280',
                  color: '#fff',
                  borderRadius: 4,
                  padding: '2px 8px',
                  fontSize: 11,
                }}
              >
                {wh.status}
              </span>
            </td>
            <td style={td}>
              <code style={{ fontSize: 12, color: '#9c9c9c' }}>{wh.secret_hint}</code>
            </td>
            <td style={td}>{wh.failure_streak}</td>
            <td style={td}>
              {wh.status === 'circuit_open' ? (
                <button style={circuitBtn} onClick={() => onCircuitToggle(wh.id, false)}>
                  Close Circuit
                </button>
              ) : (
                <button style={circuitBtn} onClick={() => onCircuitToggle(wh.id, true)}>
                  Open Circuit
                </button>
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

const th: React.CSSProperties = { padding: '4px 8px', fontWeight: 500 }
const td: React.CSSProperties = { padding: '6px 8px', verticalAlign: 'middle' }
const circuitBtn: React.CSSProperties = {
  fontSize: 11,
  background: '#056bff',
  color: '#fff',
  border: 'none',
  borderRadius: 4,
  padding: '3px 10px',
  cursor: 'pointer',
}
