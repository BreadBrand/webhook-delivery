import { useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { redeliver, setCircuit } from './api'
import { DeliveryLog } from './components/DeliveryLog'
import { EndpointHealth } from './components/EndpointHealth'
import { EventFeed } from './components/EventFeed'
import { VolumeChart } from './components/VolumeChart'
import { WebhookRegistry } from './components/WebhookRegistry'
import { useHydration } from './hooks/useHydration'
import { useSSE } from './hooks/useSSE'
import { useDashboardStore } from './store'

export default function App() {
  useHydration()
  useSSE()

  const apiKey = useDashboardStore((s) => s.apiKey)
  const webhooks = useDashboardStore((s) => s.webhooks)
  const events = useDashboardStore((s) => s.events)
  const deliveries = useDashboardStore((s) => s.deliveries)
  const volumeData = useDashboardStore((s) => s.volumeData)
  const volumeWindow = useDashboardStore((s) => s.volumeWindow)
  const setVolumeWindow = useDashboardStore((s) => s.setVolumeWindow)
  const applySSEEvent = useDashboardStore((s) => s.applySSEEvent)

  const queryClient = useQueryClient()

  const handleCircuitToggle = useCallback(
    async (id: string, open: boolean) => {
      const updated = await setCircuit(apiKey, id, open)
      applySSEEvent('webhook_updated', updated)
    },
    [apiKey, applySSEEvent],
  )

  const handleRedeliver = useCallback(
    async (id: string) => {
      await redeliver(apiKey, id)
      queryClient.invalidateQueries({ queryKey: ['deliveries'] })
    },
    [apiKey, queryClient],
  )

  const handleWindowChange = useCallback(
    (w: typeof volumeWindow) => {
      setVolumeWindow(w)
    },
    [setVolumeWindow],
  )

  return (
    <div style={{ maxWidth: 1400, margin: '0 auto', padding: '24px 16px' }}>
      <header style={{ marginBottom: 32, display: 'flex', alignItems: 'center', gap: 12 }}>
        <div
          style={{
            width: 8,
            height: 32,
            borderRadius: 4,
            background: 'linear-gradient(180deg, #6601e8 0%, #ac00d7 50%, #ff00b8 100%)',
            flexShrink: 0,
          }}
        />
        <h1
          style={{
            fontSize: 22,
            fontWeight: 700,
            background: 'linear-gradient(90deg, #6601e8, #ac00d7, #ff00b8)',
            WebkitBackgroundClip: 'text',
            WebkitTextFillColor: 'transparent',
            backgroundClip: 'text',
          }}
        >
          Webhook Delivery Dashboard
        </h1>
      </header>

      <div style={{ display: 'grid', gap: 24 }}>
        <Panel title="Webhook Registry">
          <WebhookRegistry
            webhooks={webhooks}
            apiKey={apiKey}
            onCircuitToggle={handleCircuitToggle}
          />
        </Panel>

        <Panel title="Endpoint Health">
          <EndpointHealth webhooks={webhooks} deliveries={deliveries} />
        </Panel>

        <Panel title="Event Volume">
          <VolumeChart
            data={volumeData}
            window={volumeWindow}
            onWindowChange={handleWindowChange}
          />
        </Panel>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24 }}>
          <Panel title="Recent Events">
            <EventFeed events={events} />
          </Panel>
          <Panel title="Delivery Log">
            <DeliveryLog
              deliveries={deliveries}
              apiKey={apiKey}
              onRedeliver={handleRedeliver}
            />
          </Panel>
        </div>
      </div>
    </div>
  )
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section
      style={{
        background: '#0d1635',
        border: '1px solid #2c456d',
        borderRadius: 10,
        padding: 20,
      }}
    >
      <h2
        style={{
          fontSize: 12,
          fontWeight: 600,
          color: '#9c9c9c',
          marginBottom: 16,
          textTransform: 'uppercase',
          letterSpacing: '0.08em',
        }}
      >
        {title}
      </h2>
      {children}
    </section>
  )
}
