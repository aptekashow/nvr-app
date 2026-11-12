import { useEffect, useMemo, useRef, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { getCameras, CameraStatus } from '../api'
import './Archive.css'

interface TimelineInfo {
  day: string
  filename: string
  duration: number
  thumbs: number[]
}

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

function formatTime(sec: number): string {
  const s = Math.max(0, Math.floor(sec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const r = s % 60
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(r).padStart(2, '0')}`
  return `${m}:${String(r).padStart(2, '0')}`
}

export default function Archive() {
  const [cameras, setCameras] = useState<CameraStatus[]>([])
  const [cameraId, setCameraId] = useState('')
  const [days, setDays] = useState<string[]>([])
  const [day, setDay] = useState('')
  const [timeline, setTimeline] = useState<TimelineInfo | null>(null)
  const [startSec, setStartSec] = useState(0)
  const [endSec, setEndSec] = useState(60)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [exporting, setExporting] = useState(false)
  const [hoverSec, setHoverSec] = useState<number | null>(null)
  const trackRef = useRef<HTMLDivElement>(null)
  const dragging = useRef<'start' | 'end' | null>(null)

  useEffect(() => {
    getCameras()
      .then((list: CameraStatus[]) => {
        setCameras(list)
        if (list.length) setCameraId(list[0].id)
      })
      .catch((e) => setError(e.message))
  }, [])

  useEffect(() => {
    if (!cameraId) return
    setDays([])
    setDay('')
    setTimeline(null)
    fetch(`/api/archive/${cameraId}/days`, { headers: authHeaders() })
      .then(async (r) => {
        if (!r.ok) throw new Error('Не удалось загрузить дни')
        return r.json()
      })
      .then((d: string[]) => {
        const list = d || []
        setDays(list)
        if (list.length) setDay(list[0])
      })
      .catch((e) => setError(e.message))
  }, [cameraId])

  useEffect(() => {
    if (!cameraId || !day) return
    setLoading(true)
    setError('')
    fetch(`/api/archive/${cameraId}/timeline?day=${encodeURIComponent(day)}`, { headers: authHeaders() })
      .then(async (r) => {
        if (!r.ok) {
          const err = await r.json().catch(() => ({}))
          throw new Error(err.error || 'Нет записи за этот день')
        }
        return r.json()
      })
      .then((info: TimelineInfo) => {
        setTimeline(info)
        setStartSec(0)
        setEndSec(Math.min(60, info.duration || 60))
      })
      .catch((e) => {
        setTimeline(null)
        setError(e.message)
      })
      .finally(() => setLoading(false))
  }, [cameraId, day])

  const duration = timeline?.duration || 0

  const secFromClientX = useCallback((clientX: number) => {
    const el = trackRef.current
    if (!el || duration <= 0) return 0
    const rect = el.getBoundingClientRect()
    const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width))
    return ratio * duration
  }, [duration])

  useEffect(() => {
    const onMove = (e: PointerEvent) => {
      if (!dragging.current) return
      const sec = secFromClientX(e.clientX)
      if (dragging.current === 'start') setStartSec(Math.min(sec, endSec - 1))
      else setEndSec(Math.max(sec, startSec + 1))
    }
    const onUp = () => { dragging.current = null }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [secFromClientX, startSec, endSec])

  const thumbUrl = (offset: number) => {
    if (!cameraId || !day) return ''
    return `/api/archive/${cameraId}/thumb?day=${encodeURIComponent(day)}&offset=${offset.toFixed(1)}`
  }

  const startPct = duration > 0 ? (startSec / duration) * 100 : 0
  const endPct = duration > 0 ? (endSec / duration) * 100 : 0

  const handleExport = async () => {
    if (!cameraId || !day || endSec <= startSec) return
    setExporting(true)
    setError('')
    try {
      const res = await fetch('/api/export', {
        method: 'POST',
        headers: { ...authHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ camera_id: cameraId, day, start_sec: startSec, end_sec: endSec }),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error(err.error || 'Ошибка экспорта')
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `export_${day}_${formatTime(startSec).replace(/:/g, '-')}_${formatTime(endSec).replace(/:/g, '-')}.mp4`
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch (e: any) {
      setError(e.message || 'Ошибка экспорта')
    } finally {
      setExporting(false)
    }
  }

  const thumbs = useMemo(() => timeline?.thumbs || [], [timeline])

  return (
    <div className="archive-page">
      <header className="header">
        <div className="logo">NVR</div>
        <nav>
          <Link to="/" className="nav-btn">Потоки</Link>
          <button className="nav-btn active">Архив</button>
        </nav>
        <div className="header-actions">
          <select className="select" value={cameraId} onChange={(e) => setCameraId(e.target.value)}>
            {cameras.map((c) => (<option key={c.id} value={c.id}>{c.name}</option>))}
          </select>
          <select className="select" value={day} onChange={(e) => setDay(e.target.value)} disabled={!days.length}>
            {days.length === 0 && <option value="">Нет записей</option>}
            {days.map((d) => (<option key={d} value={d}>{d}</option>))}
          </select>
          <Link to="/" className="btn">← Назад</Link>
        </div>
      </header>

      <main className="timeline-page">
        {error && <div className="error-box">{error}</div>}
        {loading && <div className="muted center">Загрузка таймлайна…</div>}

        {!loading && timeline && (
          <>
            <div className="range-info">
              <span>
                Фрагмент: <strong>{formatTime(startSec)}</strong>
                {' — '}
                <strong>{formatTime(endSec)}</strong>
                <span className="muted"> ({formatTime(endSec - startSec)})</span>
              </span>
              <button className="btn primary" disabled={exporting || endSec <= startSec} onClick={handleExport}>
                {exporting ? 'Вырезаем…' : 'Скачать фрагмент'}
              </button>
            </div>

            <div className="filmstrip">
              {thumbs.map((t) => (
                <div key={t} className="film-cell" style={{ opacity: t >= startSec && t <= endSec ? 1 : 0.35 }} title={formatTime(t)}>
                  <img src={thumbUrl(t)} alt="" loading="lazy" draggable={false} />
                  <span className="film-time">{formatTime(t)}</span>
                </div>
              ))}
            </div>

            <div
              className="timeline-track"
              ref={trackRef}
              onPointerMove={(e) => { if (!dragging.current) setHoverSec(secFromClientX(e.clientX)) }}
              onPointerLeave={() => setHoverSec(null)}
              onPointerDown={(e) => {
                const sec = secFromClientX(e.clientX)
                if (Math.abs(sec - startSec) <= Math.abs(sec - endSec)) {
                  dragging.current = 'start'
                  setStartSec(Math.min(sec, endSec - 1))
                } else {
                  dragging.current = 'end'
                  setEndSec(Math.max(sec, startSec + 1))
                }
              }}
            >
              <div className="track-bg" />
              <div className="track-sel" style={{ left: `${startPct}%`, width: `${endPct - startPct}%` }} />
              {hoverSec !== null && !dragging.current && (
                <div className="hover-line" style={{ left: `${(hoverSec / duration) * 100}%` }}>
                  <div className="hover-preview">
                    <img src={thumbUrl(hoverSec)} alt="" />
                    <span>{formatTime(hoverSec)}</span>
                  </div>
                </div>
              )}
              <div className="handle start" style={{ left: `${startPct}%` }} onPointerDown={(e) => { e.stopPropagation(); dragging.current = 'start' }}>
                <div className="handle-knob" />
                <div className="handle-label">{formatTime(startSec)}</div>
              </div>
              <div className="handle end" style={{ left: `${endPct}%` }} onPointerDown={(e) => { e.stopPropagation(); dragging.current = 'end' }}>
                <div className="handle-knob" />
                <div className="handle-label">{formatTime(endSec)}</div>
              </div>
            </div>

            <p className="hint">
              Потяните ползунки, чтобы выбрать начало и конец. Скачивание через <code>ffmpeg -c copy</code> — без перекодирования.
            </p>
          </>
        )}

        {!loading && !timeline && !error && (
          <div className="muted center">
            Выберите камеру и день с записью. Запись включается на странице «Потоки».
          </div>
        )}
      </main>
    </div>
  )
}
