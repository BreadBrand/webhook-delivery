import { useState } from 'react'
import type { Event } from '../types'

interface Props {
  events: Event[]
}

function truncate(v: unknown, max = 60): string {
  const s = JSON.stringify(v) ?? ''
  return s.length > max ? s.slice(0, max) + '…' : s
}

export function EventFeed({ events }: Props) {
  const [search, setSearch] = useState('')

  const filtered = search
    ? events.filter((ev) => {
        const q = search.toLowerCase()
        return (
          ev.type.toLowerCase().includes(q) ||
          ev.source.toLowerCase().includes(q) ||
          JSON.stringify(ev.data).toLowerCase().includes(q)
        )
      })
    : events

  return (
    <div>
      <input
        type="search"
        placeholder="Search by type, source, or data…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={searchInput}
      />
      {filtered.length === 0 ? (
        <p style={{ color: '#9c9c9c', padding: '12px 0', fontSize: 13 }}>
          {events.length === 0 ? 'No events received yet.' : 'No events match your search.'}
        </p>
      ) : (
        <ul
          style={{
            listStyle: 'none',
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
            marginTop: 12,
            maxHeight: 360,
            overflowY: 'auto',
            paddingRight: 4,
          }}
        >
          {filtered.map((ev) => (
            <li
              key={ev.id}
              style={{
                background: '#182945',
                border: '1px solid #2c456d',
                borderRadius: 6,
                padding: '8px 12px',
                fontSize: 13,
              }}
            >
              <div style={{ display: 'flex', gap: 12, alignItems: 'baseline' }}>
                <span style={{ color: '#056bff', fontWeight: 600 }}>{ev.type}</span>
                <span style={{ color: '#9c9c9c', fontSize: 11 }}>{ev.source}</span>
                <span style={{ color: '#44505a', fontSize: 11, marginLeft: 'auto' }}>
                  {new Date(ev.time).toLocaleTimeString()}
                </span>
              </div>
              <div style={{ color: '#44505a', fontSize: 11, marginTop: 2, fontFamily: 'monospace' }}>
                {truncate(ev.data)}
              </div>
            </li>
          ))}
        </ul>
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
