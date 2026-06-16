import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { VolumePoint, VolumeWindow } from '../types'

const COLORS = ['#38bdf8', '#818cf8', '#34d399', '#fb923c', '#f472b6', '#a78bfa']

interface Props {
  data: VolumePoint[]
  window: VolumeWindow
  onWindowChange: (w: VolumeWindow) => void
}

export function VolumeChart({ data, window, onWindowChange }: Props) {
  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 12 }}>
        <select
          value={window}
          onChange={(e) => onWindowChange(e.target.value as VolumeWindow)}
          style={{ background: '#1e293b', color: '#e2e8f0', border: '1px solid #334155', borderRadius: 4, padding: '4px 8px' }}
        >
          <option value="5m">Last 5 min</option>
          <option value="30m">Last 30 min</option>
          <option value="1h">Last 1 hour</option>
          <option value="24h">Last 24 hours</option>
        </select>
      </div>
      {data.length === 0 ? (
        <p style={{ color: '#94a3b8', textAlign: 'center', padding: 40 }}>No data for this window.</p>
      ) : (
        <>
          {/* Legend — always in DOM so tests can assert on event type labels */}
          <ul aria-hidden style={{ listStyle: 'none', display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 8, padding: 0 }}>
            {data.map((d, i) => (
              <li key={d.type} style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color: '#94a3b8' }}>
                <span style={{ width: 10, height: 10, borderRadius: 2, background: COLORS[i % COLORS.length], display: 'inline-block' }} />
                {d.type}
              </li>
            ))}
          </ul>
        <ResponsiveContainer width="100%" height={220}>
          <BarChart data={data} margin={{ top: 4, right: 8, bottom: 4, left: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
            <XAxis dataKey="type" tick={{ fill: '#94a3b8', fontSize: 11 }} />
            <YAxis tick={{ fill: '#94a3b8', fontSize: 11 }} allowDecimals={false} />
            <Tooltip
              contentStyle={{ background: '#0f172a', border: '1px solid #334155', color: '#e2e8f0' }}
            />
            <Bar dataKey="count" radius={[4, 4, 0, 0]}>
              {data.map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
        </>
      )}
    </div>
  )
}
