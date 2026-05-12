import { useState, useEffect } from 'react'

export default function MonitorSettingsSection({ t, authFetch, onMessage }) {
    const apiFetch = authFetch || fetch
    const [form, setForm] = useState(null)
    const [loading, setLoading] = useState(true)
    const [saving, setSaving] = useState(false)
    const [testing, setTesting] = useState(false)

    useEffect(() => { loadMonitorSettings() }, [])

    const loadMonitorSettings = async () => {
        setLoading(true)
        try {
            const res = await apiFetch('/admin/monitor/settings')
            const data = await res.json()
            if (res.ok) setForm(data)
        } catch (_e) { }
        if (!form) {
            setForm({
                metrics: { enabled: true, path: '/metrics' },
                alerting: {
                    enabled: true, rate_limit_seconds: 60,
                    channels: {
                        discord: { enabled: false, webhook_url: '' },
                        slack: { enabled: false, webhook_url: '' },
                        telegram: { enabled: false, bot_token: '', chat_id: '' },
                        custom: { enabled: false, url: '', headers: {} }
                    },
                    triggers: {
                        account_all_down: true, high_error_rate: true, high_error_rate_threshold: 0.30,
                        consecutive_upstream_failures: true, consecutive_upstream_threshold: 10,
                        session_creation_failure: true, pow_failure: true,
                        content_filter_block: true, token_refresh_failure: true
                    }
                }
            })
        }
        setLoading(false)
    }

    const saveMonitorSettings = async () => {
        setSaving(true)
        try {
            const res = await apiFetch('/admin/monitor/settings', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(form),
            })
            const data = await res.json()
            if (!res.ok) { onMessage?.('error', data.detail || 'Failed to save.'); return }
            onMessage?.('success', t('monitor.saved'))
        } catch (_e) { onMessage?.('error', t('monitor.saveFailed')) }
        finally { setSaving(false) }
    }

    const testAlert = async () => {
        setTesting(true)
        try {
            const res = await apiFetch('/admin/monitor/health-check')
            const data = await res.json()
            onMessage?.('success', `${data.healthy ? '✅' : '❌'} ${data.accounts_ok} OK / ${data.accounts_down} DOWN`)
        } catch (_e) { onMessage?.('error', t('monitor.healthCheckFailed')) }
        finally { setTesting(false) }
    }

    if (loading || !form) return <div className="bg-card border border-border rounded-xl p-5 animate-pulse"><div className="h-5 w-32 bg-muted rounded" /></div>

    const update = (path, value) => {
        setForm(prev => {
            const next = JSON.parse(JSON.stringify(prev))
            const keys = path.split('.')
            let obj = next
            for (let i = 0; i < keys.length - 1; i++) obj = obj[keys[i]]
            obj[keys[keys.length - 1]] = value
            return next
        })
    }

    return (
        <div className="bg-card border border-border rounded-xl p-5 space-y-6">
            <div className="flex items-center justify-between">
                <h3 className="font-semibold">{t('monitor.title')}</h3>
                <div className="flex gap-2">
                    <button type="button" onClick={testAlert} disabled={testing}
                        className="px-3 py-1.5 text-xs rounded-lg border border-border bg-secondary hover:bg-secondary/80">
                        {testing ? '...' : t('monitor.testAlert')}
                    </button>
                    <button type="button" onClick={saveMonitorSettings} disabled={saving}
                        className="px-3 py-1.5 text-xs rounded-lg bg-primary text-primary-foreground hover:bg-primary/90">
                        {saving ? t('monitor.saving') : t('monitor.save')}
                    </button>
                </div>
            </div>

            {/* Prometheus Metrics */}
            <div className="space-y-3 border-t border-border pt-4">
                <h4 className="text-sm font-medium">{t('monitor.metricsSection')}</h4>
                <label className="flex items-center gap-3">
                    <input type="checkbox" checked={form.metrics?.enabled ?? true}
                        onChange={(e) => update('metrics.enabled', e.target.checked)} className="h-4 w-4 rounded border-border" />
                    <span className="text-sm">{t('monitor.metricsEnabled')}</span>
                </label>
                {form.metrics?.enabled !== false && (
                    <label className="text-sm space-y-1 block">
                        <span className="text-muted-foreground">{t('monitor.metricsPath')}</span>
                        <input type="text" value={form.metrics?.path || '/metrics'}
                            onChange={(e) => update('metrics.path', e.target.value)}
                            className="w-full max-w-xs bg-background border border-border rounded-lg px-3 py-2" />
                    </label>
                )}
            </div>

            {/* Alerting */}
            <div className="space-y-3 border-t border-border pt-4">
                <h4 className="text-sm font-medium">{t('monitor.alertingSection')}</h4>
                <label className="flex items-center gap-3">
                    <input type="checkbox" checked={form.alerting?.enabled ?? true}
                        onChange={(e) => update('alerting.enabled', e.target.checked)} className="h-4 w-4 rounded border-border" />
                    <span className="text-sm">{t('monitor.alertingEnabled')}</span>
                </label>
                {form.alerting?.enabled !== false && (<>
                    <label className="text-sm space-y-1 block">
                        <span className="text-muted-foreground">{t('monitor.rateLimitSeconds')}</span>
                        <input type="number" min={10} max={3600} value={form.alerting?.rate_limit_seconds || 60}
                            onChange={(e) => update('alerting.rate_limit_seconds', Number(e.target.value || 60))}
                            className="w-full max-w-xs bg-background border border-border rounded-lg px-3 py-2" />
                    </label>

                    <ChannelConfig t={t} channel="discord" label="Discord" fields={[
                        { key: 'enabled', label: t('monitor.channelEnabled'), type: 'checkbox' },
                        { key: 'webhook_url', label: t('monitor.webhookUrl'), type: 'text', placeholder: 'https://discord.com/api/webhooks/...' }
                    ]} form={form} update={update} />

                    <ChannelConfig t={t} channel="slack" label="Slack" fields={[
                        { key: 'enabled', label: t('monitor.channelEnabled'), type: 'checkbox' },
                        { key: 'webhook_url', label: t('monitor.webhookUrl'), type: 'text', placeholder: 'https://hooks.slack.com/services/...' }
                    ]} form={form} update={update} />

                    <ChannelConfig t={t} channel="telegram" label="Telegram" fields={[
                        { key: 'enabled', label: t('monitor.channelEnabled'), type: 'checkbox' },
                        { key: 'bot_token', label: t('monitor.botToken'), type: 'text', placeholder: '123456:ABC-DEF...' },
                        { key: 'chat_id', label: t('monitor.chatId'), type: 'text', placeholder: '-100123456' }
                    ]} form={form} update={update} />

                    <ChannelConfig t={t} channel="custom" label={t('monitor.customWebhook')} fields={[
                        { key: 'enabled', label: t('monitor.channelEnabled'), type: 'checkbox' },
                        { key: 'url', label: t('monitor.webhookUrl'), type: 'text', placeholder: 'https://your-service.com/webhook' }
                    ]} form={form} update={update} />

                    <div className="space-y-3 border-t border-border pt-4">
                        <h5 className="text-sm font-medium text-muted-foreground">{t('monitor.triggersSection')}</h5>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                            {[
                                { key: 'account_all_down', label: t('monitor.triggerAllAccountsDown') },
                                { key: 'high_error_rate', label: t('monitor.triggerHighErrorRate'), extra: { key: 'high_error_rate_threshold', step: 0.05, min: 0.05, max: 1 } },
                                { key: 'consecutive_upstream_failures', label: t('monitor.triggerConsecutiveFailures'), extra: { key: 'consecutive_upstream_threshold', step: 1, min: 2, max: 100 } },
                                { key: 'session_creation_failure', label: t('monitor.triggerSessionFailure') },
                                { key: 'pow_failure', label: t('monitor.triggerPowFailure') },
                                { key: 'content_filter_block', label: t('monitor.triggerContentFilter') },
                                { key: 'token_refresh_failure', label: t('monitor.triggerTokenRefresh') },
                            ].map(trigger => (
                                <div key={trigger.key} className="flex items-center gap-3">
                                    <input type="checkbox" checked={form.alerting?.triggers?.[trigger.key] ?? true}
                                        onChange={(e) => update(`alerting.triggers.${trigger.key}`, e.target.checked)}
                                        className="h-4 w-4 rounded border-border" />
                                    <span className="text-sm">{trigger.label}</span>
                                    {trigger.extra && form.alerting?.triggers?.[trigger.key] && (
                                        <input type="number" step={trigger.extra.step} min={trigger.extra.min} max={trigger.extra.max}
                                            value={form.alerting?.triggers?.[trigger.extra.key] || 30}
                                            onChange={(e) => update(`alerting.triggers.${trigger.extra.key}`, Number(e.target.value))}
                                            className="ml-auto w-16 bg-background border border-border rounded px-2 py-1 text-sm text-right" />
                                    )}
                                </div>
                            ))}
                        </div>
                    </div>
                </>)}
            </div>
        </div>
    )
}

function ChannelConfig({ t, channel, label, fields, form, update }) {
    const ch = form?.alerting?.channels?.[channel]
    if (!ch) return null
    return (
        <details className="border border-border rounded-lg p-3">
            <summary className="flex items-center gap-2 cursor-pointer">
                <span className="text-sm font-medium">{label}</span>
                <span className={`ml-auto w-2 h-2 rounded-full ${ch.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/30'}`} />
            </summary>
            <div className="space-y-3 mt-3 pt-3 border-t border-border">
                {fields.map(field => (
                    <label key={field.key} className="text-sm space-y-1 block">
                        {field.type === 'checkbox' ? (
                            <div className="flex items-center gap-3">
                                <input type="checkbox" checked={ch[field.key] ?? false}
                                    onChange={(e) => update(`alerting.channels.${channel}.${field.key}`, e.target.checked)}
                                    className="h-4 w-4 rounded border-border" />
                                <span>{field.label}</span>
                            </div>
                        ) : (
                            <><span className="text-muted-foreground">{field.label}</span>
                                <input type={field.type || 'text'} value={ch[field.key] || ''} placeholder={field.placeholder || ''}
                                    onChange={(e) => update(`alerting.channels.${channel}.${field.key}`, e.target.value)}
                                    className="w-full bg-background border border-border rounded-lg px-3 py-2 font-mono text-sm" /></>
                        )}
                    </label>
                ))}
            </div>
        </details>
    )
}
