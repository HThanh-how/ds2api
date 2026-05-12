import { useI18n } from '../../i18n'

export default function UsageContainer({ authFetch, onMessage }) {
    const { t } = useI18n()
    return (
        <div className="bg-card border border-border rounded-xl p-12 text-center">
            <div className="text-4xl mb-4">📊</div>
            <h3 className="text-lg font-semibold mb-2">{t('usage.title')}</h3>
            <p className="text-sm text-muted-foreground">{t('usage.comingSoon')}</p>
        </div>
    )
}
