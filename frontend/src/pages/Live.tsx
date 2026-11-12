import { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import Hls from 'hls.js'
import { getCameras, CameraStatus } from '../api'
import './Live.css'

const HLS_MIME = 'application/vnd.apple.mpegurl'

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
    let nativeErrorHandler: (() => void) | undefined
    let nativeMetadataHandler: (() => void) | undefined
    const src = cam.hls_url
    setError('')

    const isHevcStream = () =>
      Boolean(hls?.levels.some((level) => /^(hvc1|hev1)/i.test(level.videoCodec || '')))

    const hevcNotSupported = () => {
      if (!hls || typeof MediaSource === 'undefined') return false
      return hls.levels.some((level) => {
        const codec = level.videoCodec || ''
        return /^(hvc1|hev1)/i.test(codec) &&
          !MediaSource.isTypeSupported(`video/mp4; codecs="${codec}"`)
      })
    }

    const showHevcHelp = () => {
      setError('Камера передаёт H.265/HEVC, но этот браузер или устройство не поддерживает его аппаратное/программное декодирование. Попробуйте Safari на совместимом устройстве или браузер с поддержкой HEVC.')
    }

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

    const onNativeMetadata = () => {
      video.play().catch(() => setError('Нажмите ▶ для запуска видео'))
    }
    const onNativeError = () => {
      retry('Браузер не смог открыть HLS-поток. Проверьте загрузку плейлиста, сегментов и поддержку кодека')
    }

    // Prefer native HLS where available (notably Safari), as its platform
    // decoder often has broader HEVC support than Media Source Extensions.
    // Prefer hls.js when MSE is available. Some Chromium builds report
    // native HLS support inconsistently and then fail without useful details.
    // Use native HLS only as a fallback (notably Safari/iOS).
    if (!Hls.isSupported() && video.canPlayType(HLS_MIME)) {
      nativeMetadataHandler = onNativeMetadata
      nativeErrorHandler = onNativeError
      video.addEventListener('loadedmetadata', onNativeMetadata, { once: true })
      video.addEventListener('error', onNativeError)
      video.src = src
      video.load()
    } else if (Hls.isSupported()) {
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
        if (hevcNotSupported()) {
          showHevcHelp()
          hls?.destroy()
          hls = null
          return
        }
        setError('')
        retries = 0
        video.play().catch(() => setError('Нажмите ▶ для запуска видео'))
      })
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal || disposed) return
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          if (isHevcStream() && hevcNotSupported()) {
            showHevcHelp()
            hls?.destroy()
            hls = null
            return
          }
          hls?.recoverMediaError()
          return
        }
        retry(data.details || 'Поток пока недоступен')
      })
    } else {
      setError('Этот браузер не поддерживает HLS. Для H.265/HEVC также требуется поддержка кодека устройством.')
    }

    return () => {
      disposed = true
      if (retryTimer) clearTimeout(retryTimer)
      if (hls) hls.destroy()
      if (nativeMetadataHandler) video.removeEventListener('loadedmetadata', nativeMetadataHandler)
      if (nativeErrorHandler) video.removeEventListener('error', nativeErrorHandler)
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
