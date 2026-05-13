import { useState, useEffect, useRef, useCallback } from 'react'

export function useAccountHealth({ apiFetch, pollInterval = 10000 }) {
    const [healthData, setHealthData] = useState({})
    const timerRef = useRef(null)

    const fetchHealth = useCallback(async () => {
        try {
            const resp = await apiFetch('/admin/accounts/health')
            if (!resp.ok) return
            const data = await resp.json()
            const map = {}
            for (const item of data) {
                map[item.account_id] = item
            }
            setHealthData(map)
        } catch {
            // silent
        }
    }, [apiFetch])

    useEffect(() => {
        fetchHealth()
        timerRef.current = setInterval(fetchHealth, pollInterval)
        return () => clearInterval(timerRef.current)
    }, [fetchHealth, pollInterval])

    return { healthData, refreshHealth: fetchHealth }
}

export function formatCooldownTime(cooldownUntil) {
    if (!cooldownUntil || cooldownUntil <= 0) return null
    const now = Math.floor(Date.now() / 1000)
    const diff = cooldownUntil - now
    if (diff <= 0) return null
    const hours = Math.floor(diff / 3600)
    const minutes = Math.floor((diff % 3600) / 60)
    if (hours > 0) return `${hours}h ${minutes}m`
    if (minutes > 0) return `${minutes}m`
    return `${diff}s`
}

export function healthStatusLabel(status) {
    switch (status) {
        case 'healthy': return 'Healthy'
        case 'muted': return 'Muted'
        case 'rate_limited': return 'Rate Limited'
        case 'login_failed': return 'Login Failed'
        case 'cooldown': return 'Cooldown'
        default: return 'Unknown'
    }
}

export function healthStatusColor(status) {
    switch (status) {
        case 'healthy': return 'emerald'
        case 'muted': return 'red'
        case 'rate_limited': return 'amber'
        case 'login_failed': return 'gray'
        case 'cooldown': return 'blue'
        default: return 'gray'
    }
}
