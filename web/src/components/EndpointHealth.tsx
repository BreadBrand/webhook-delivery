import type { Delivery, Webhook } from '../types'

interface Props {
  webhooks: Webhook[]
  deliveries: Delivery[]
}

interface Stats {
  successRate: number
  avgResponseMs: number | null
  lastDeliveryAt: string | null
}

function computeStats(webhookId: string, deliveries: Delivery[]): Stats {
  const mine = deliveries.filter((d) => d.webhook_id === webhookId)
  if (mine.length === 0) return { successRate: 0, avgResponseMs: null, lastDeliveryAt: null }

  const successes = mine.filter((d) => d.status === 'success').length
  const successRate = Math.round((successes / mine.length) * 100)

  const withMs = mine.filter((d) => d.last_response_ms != null)
  const avgResponseMs =
    withMs.length > 0
      ? Math.round(withMs.reduce((s, d) => s + d.last_response_ms!, 0) / withMs.length)
      : null

  const sorted = [...mine].sort(
    (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
  )
  const lastDeliveryAt = sorted[0]?.updated_at ?? null

  return { successRate, avgResponseMs, lastDeliveryAt }
}

const statusColor: Record<string, string> = {
  active: '#22c55e',
  degraded: '#f59e0b',
  circuit_open: '#ef4444',
  deleted: '#6b7280',
}

export function EndpointHealth({ webhooks, deliveries }: Props) {
  if (webhooks.length === 0) {
    return <p style={{ color: '#94a3b8' }}>No webhooks registered yet.</p>
  }

  return (
    <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))' }}>
      {webhooks.map((wh) => {
        const { successRate, avgResponseMs, lastDeliveryAt } = computeStats(wh.id, deliveries)
        return (
          <div
            key={wh.id}
            style={{
              background: '#1e293b',
              borderRadius: 8,
              padding: '12px 16px',
              fontSize: 13,
            }}
          >
            <div
              style={{
                color: '#38bdf8',
                fontSize: 12,
                fontFamily: 'monospace',
                marginBottom: 8,
                wordBreak: 'break-all',
              }}
            >
              {wh.url}
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6 }}>
              <Stat label="Success rate" value={`${successRate}%`} />
              <Stat label="Failure streak" value={String(wh.failure_streak)} />
              <Stat
                label="Avg latency"
                value={avgResponseMs != null ? `${avgResponseMs}ms` : '—'}
              />
              <Stat
                label="Circuit"
                value={wh.status}
                color={statusColor[wh.status] ?? '#6b7280'}
              />
            </div>
            {lastDeliveryAt && (
              <div style={{ color: '#475569', fontSize: 11, marginTop: 8 }}>
                Last delivery: {new Date(lastDeliveryAt).toLocaleString()}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

function Stat({
  label,
  value,
  color,
}: {
  label: string
  value: string
  color?: string
}) {
  return (
    <div>
      <div style={{ color: '#64748b', fontSize: 10, textTransform: 'uppercase' }}>{label}</div>
      <div style={{ color: color ?? '#e2e8f0', fontWeight: 600 }}>{value}</div>
    </div>
  )
}
