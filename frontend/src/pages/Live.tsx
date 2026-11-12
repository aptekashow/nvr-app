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
    if (!cam?.hls_url || !videoRef.current) return

    const video = videoRef.current
    let hls: Hls | null = null

    if (Hls.isSupported()) {
      hls = new Hls({ enableWorker: true, lowLatencyMode: true })
      hls.loadSource(cam.hls_url)
      hls.attachMedia(video)
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {})
      })
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) setError('Ошибка воспроизведения потока')
      })
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = cam.hls_url
      video.play().catch(() => {})
    } else {
      setError('HLS не поддерживается в этом браузере')
    }

    return () => {
      if (hls) hls.destroy()
    }
  }, [cam?.hls_url])

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
        <div className="live-status">
          <span className={`dot ${cam.online ? 'online' : 'offline'}`} />
          {cam.online ? 'В сети' : 'Не в сети'}
          {cam.recording && <span className="rec">● REC</span>}
        </div>
        <div className="zoom-info">{scale.toFixed(1)}x</div>
      </div>

      <div className="player-wrap" ref={containerRef} onDoubleClick={resetZoom}>
        {error ? (
          <div className="live-error">{error}</div>
        ) : (
          <video
            ref={videoRef}
            className="player"
            style={{ transform: `scale(${scale})` }}
            muted
            playsInline
            controls={false}
          />
        )}
      </div>

      <div className="live-hint">
        Колёсико мыши — зум · Двойной клик — сброс
      </div>
    </div>
  )
}
