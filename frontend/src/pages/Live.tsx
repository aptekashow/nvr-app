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
  const [scale, setScale] = useState(1)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!id) return
    getCameras().then((list: CameraStatus[]) => {
      const found = list.find((c) => c.id === id)
      setCam(found || null)
    })
  }, [id])

  useEffect(() => {
    if (!cam || !videoRef.current) return

    const video = videoRef.current
    let hls: Hls | null = null
    const src = cam.hls_url
    setError('')

    if (Hls.isSupported()) {
      hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
      })
      hls.loadSource(src)
      hls.attachMedia(video)
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {})
      })
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) {
          setError(`Ошибка потока: ${src}`)
        }
      })
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = src
      video.play().catch(() => {})
    } else {
      setError('HLS не поддерживается в этом браузере')
    }

    return () => {
      if (hls) hls.destroy()
    }
  }, [cam?.id])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      setScale((prev) => {
        const next = prev - e.deltaY * 0.0015
        return Math.min(Math.max(next, 1), 5)
      })
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  const resetZoom = () => setScale(1)

  if (!cam) {
    return (
      <div className="live-page">
        <div className="live-header">
          <Link to="/" className="back">← Назад</Link>
        </div>
        <div className="live-empty">Камера не найдена</div>
      </div>
    )
  }

  return (
    <div className="live-page">
      <div className="live-header">
        <Link to="/" className="back">← Назад</Link>
        <h1>{cam.name}</h1>
        <span className={`badge ${cam.online ? 'online' : 'offline'}`}>
          {cam.online ? 'Online' : 'Offline'}
        </span>
        <span className="zoom-hint">Колесо мыши — зум · двойной клик — сброс</span>
      </div>

      {error && <div className="live-error">{error}</div>}

      <div className="live-stage" ref={containerRef} onDoubleClick={resetZoom}>
        <video
          ref={videoRef}
          className="live-video"
          style={{ transform: `scale(${scale})` }}
          muted
          autoPlay
          playsInline
          controls={false}
        />
      </div>
    </div>
  )
}
