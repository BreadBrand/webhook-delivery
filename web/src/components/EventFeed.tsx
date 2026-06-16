import type { Event } from '../types'

interface Props {
  events: Event[]
}

function truncate(v: unknown, max = 60): string {
  const s = JSON.stringify(v) ?? ''
  return s.length > max ? s.slice(0, max) + '…' : s
}

export function EventFeed({ events }: Props) {
  if (events.length === 0) {
    return <p style={{ color: '#94a3b8' }}>No events received yet.</p>
  }

  return (
    <ul style={{ listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 8 }}>
      {events.map((ev) => (
        <li
          key={ev.id}
          style={{
            background: '#1e293b',
            borderRadius: 6,
            padding: '8px 12px',
            fontSize: 13,
          }}
        >
          <div style={{ display: 'flex', gap: 12, alignItems: 'baseline' }}>
            <span style={{ color: '#38bdf8', fontWeight: 600 }}>{ev.type}</span>
            <span style={{ color: '#94a3b8', fontSize: 11 }}>{ev.source}</span>
            <span style={{ color: '#475569', fontSize: 11, marginLeft: 'auto' }}>
              {new Date(ev.time).toLocaleTimeString()}
            </span>
          </div>
          <div style={{ color: '#64748b', fontSize: 11, marginTop: 2, fontFamily: 'monospace' }}>
            {truncate(ev.data)}
          </div>
        </li>
      ))}
    </ul>
  )
}
