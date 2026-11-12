import { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import Hls from 'hls.js'
import { getCameras, CameraStatus } from '../api'
import './Live.css'

export default function Live() {
  const { id } = useParams<{ id: string }>()
  const videoRef = useRef<HTMLVideoElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [cam, setCam] = useState<CameraStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [scale, setScale] = useState(1)
  const [error, setError] = useState('')
  const [retryKey, setRetryKey] = useState(0)

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    setCam(null)
    if (!id) return
    getCameras()
      .then((list: CameraStatus[]) => {
        if (!active) return
        const found = list.find((c) => c.id === id)
        if (!found) setError('Камера не найдена')
        setCam(found || null)
      })
      .catch((e: Error) => {
        if (active) setError(e.message || 'Не удалось загрузить список камер')
      })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [id])

  useEffect(() => {
    if (!cam?.hls_url || !videoRef.current) return
    const video = videoRef.current
    let hls: Hls | null = null
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let retries = 0
    let disposed = false
    const src = cam.hls_url
    setError('')

    const retry = (message: string) => {
      if (disposed) return
      if (retries >= 8) {
        setError(message + '. Проверьте RTSP-адрес, логин и доступность камеры.')
        return
      }
      retries += 1
      setError(message + ` (повтор ${retries}/8)…`)
      retryTimer = setTimeout(() => {
        if (disposed) return
        if (hls) {
          hls.stopLoad()
          hls.loadSource(src)
          hls.startLoad()
        } else {
          video.load()
        }
      }, Math.min(1000 * retries, 5000))
    }

    if (Hls.isSupported()) {
      hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        capLevelToPlayerSize: true,
        maxBufferLength: 12,
        backBufferLength: 30,
        liveSyncDurationCount: 3,
        liveMaxLatencyDurationCount: 6,
        manifestLoadingMaxRetry: 8,
        manifestLoadingRetryDelay: 1000,
        manifestLoadingMaxRetryTimeout: 8000,
        levelLoadingMaxRetry: 8,
        levelLoadingRetryDelay: 1000,
        fragLoadingMaxRetry: 6,
        fragLoadingRetryDelay: 1000,
      })
      hls.loadSource(src)
      hls.attachMedia(video)
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        setError('')
        retries = 0
        video.play().catch(() => setError('Нажмите ▶ для запуска видео'))
      })
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal || disposed) return
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          hls?.recoverMediaError()
          return
        }
        retry(data.details || 'Поток пока недоступен')
      })
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = src
      video.addEventListener('loadedmetadata', () => {
        video.play().catch(() => setError('Нажмите ▶ для запуска видео'))
      }, { once: true })
      video.addEventListener('error', () => retry('Не удалось открыть HLS-поток'))
      video.load()
    } else {
      setError('HLS не поддерживается этим браузером')
    }

    return () => {
      disposed = true
      if (retryTimer) clearTimeout(retryTimer)
      if (hls) hls.destroy()
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
  }, [cam?.id, cam?.hls_url, retryKey])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      setScale((prev) => Math.min(Math.max(prev - e.deltaY * 0.0015, 1), 5))
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  const resetZoom = () => setScale(1)

  return (
    <div className="live-page">
      <div className="live-header">
        <Link to="/" className="back">← Назад</Link>
        <h1>{cam?.name || 'Прямой эфир'}</h1>
        {cam && <span className={`badge ${cam.online ? 'online' : 'offline'}`}>{cam.online ? 'Online' : 'Ожидание потока'}</span>}
        <span className="zoom-hint">Колесо мыши — зум · двойной клик — сброс</span>
      </div>

      <div className="live-stage" ref={containerRef} onDoubleClick={resetZoom}>
        <video
          ref={videoRef}
          className="live-video"
          style={{ transform: `scale(${scale})` }}
          muted
          autoPlay
          playsInline
          controls
        />
        {loading && <div className="live-overlay">Загрузка камеры…</div>}
        {!loading && error && (
          <div className="live-overlay live-error-panel">
            <span>{error}</span>
            {cam && <button className="btn primary" onClick={() => { setError(''); setRetryKey((v) => v + 1) }}>Повторить подключение</button>}
          </div>
        )}
      </div>
    </div>
  )
}
