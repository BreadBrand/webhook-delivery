import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'
import type { VolumePoint, VolumeWindow } from '../types'

const COLORS = ['#056bff', '#00b6ff', '#6601e8', '#ac00d7', '#ff00b8', '#3682ff']

interface Props {
  data: VolumePoint[]
  window: VolumeWindow
  onWindowChange: (w: VolumeWindow) => void
}

export function VolumeChart({ data, window, onWindowChange }: Props) {
  const total = data.reduce((s, d) => s + d.count, 0)

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 12 }}>
        <select
          value={window}
          onChange={(e) => onWindowChange(e.target.value as VolumeWindow)}
          style={{
            background: '#182945',
            color: '#fafafa',
            border: '1px solid #2c456d',
            borderRadius: 4,
            padding: '4px 8px',
            cursor: 'pointer',
            outline: 'none',
          }}
        >
          <option value="5m">Last 5 min</option>
          <option value="30m">Last 30 min</option>
          <option value="1h">Last 1 hour</option>
          <option value="24h">Last 24 hours</option>
        </select>
      </div>

      {data.length === 0 ? (
        <p style={{ color: '#9c9c9c', textAlign: 'center', padding: 40 }}>No data for this window.</p>
      ) : (
        <div style={{ display: 'flex', alignItems: 'center', gap: 32 }}>
          <div style={{ flexShrink: 0, width: 220, height: 220, position: 'relative' }}>
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={data}
                  dataKey="count"
                  nameKey="type"
                  cx="50%"
                  cy="50%"
                  innerRadius={62}
                  outerRadius={95}
                  paddingAngle={2}
                  strokeWidth={0}
                >
                  {data.map((_, i) => (
                    <Cell key={i} fill={COLORS[i % COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip
                  contentStyle={{ background: '#0d1635', border: '1px solid #2c456d', color: '#fafafa', fontSize: 12 }}
                  formatter={(value: number) => [
                    `${value} (${Math.round((value / total) * 100)}%)`,
                    'Requests',
                  ]}
                />
              </PieChart>
            </ResponsiveContainer>
            {/* Center label */}
            <div
              style={{
                position: 'absolute',
                inset: 0,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                pointerEvents: 'none',
              }}
            >
              <span style={{ fontSize: 22, fontWeight: 700, color: '#fafafa' }}>{total}</span>
              <span style={{ fontSize: 10, color: '#9c9c9c', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                total
              </span>
            </div>
          </div>

          <ul style={{ listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 10, flex: 1 }}>
            {data.map((d, i) => {
              const pct = Math.round((d.count / total) * 100)
              return (
                <li key={d.type} style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                  <span
                    style={{
                      width: 10,
                      height: 10,
                      borderRadius: '50%',
                      background: COLORS[i % COLORS.length],
                      flexShrink: 0,
                    }}
                  />
                  <span style={{ color: '#fafafa', fontSize: 13, flex: 1 }}>{d.type}</span>
                  <span style={{ color: '#9c9c9c', fontSize: 12, fontWeight: 600 }}>{pct}%</span>
                  <span style={{ color: '#44505a', fontSize: 11 }}>({d.count})</span>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}
