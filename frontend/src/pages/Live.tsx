import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import Hls from 'hls.js'
import { getCameras, type CameraStatus } from '../api'
import './Live.css'

const HLS_MIME = 'application/vnd.apple.mpegurl'
const MAX_RETRIES = 8

type PlayerState = 'loading' | 'playing' | 'waiting' | 'error'

export default function Live() {
  const { id } = useParams<{ id: string }>()
  const videoRef = useRef<HTMLVideoElement>(null)
  const stageRef = useRef<HTMLDivElement>(null)
  const hlsRef = useRef<Hls | null>(null)
  const [camera, setCamera] = useState<CameraStatus | null>(null)
  const [pageLoading, setPageLoading] = useState(true)
  const [playerState, setPlayerState] = useState<PlayerState>('loading')
  const [message, setMessage] = useState('Подключаемся к камере…')
  const [scale, setScale] = useState(1)
  const [manualRetry, setManualRetry] = useState(0)

  useEffect(() => {
    let alive = true
    if (!id) {
      setPageLoading(false)
      setMessage('Не указан идентификатор камеры')
      setPlayerState('error')
      return
    }

    setPageLoading(true)
    getCameras()
      .then((items: CameraStatus[]) => {
        if (!alive) return
        const found = items.find((item) => item.id === id)
        setCamera(found ?? null)
        if (!found) {
          setMessage('Камера не найдена')
          setPlayerState('error')
        }
      })
      .catch((error: unknown) => {
        if (!alive) return
        setMessage(error instanceof Error ? error.message : 'Не удалось получить список камер')
        setPlayerState('error')
      })
      .finally(() => {
        if (alive) setPageLoading(false)
      })

    return () => { alive = false }
  }, [id])

  const retry = useCallback(() => {
    setMessage('Повторное подключение…')
    setPlayerState('loading')
    setManualRetry((value) => value + 1)
  }, [])

  useEffect(() => {
    const video = videoRef.current
    const source = camera?.hls_url
    if (!video || !source) return

    let disposed = false
    let hls: Hls | null = null
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let retryCount = 0
    let nativeMode = false

    const update = (state: PlayerState, text: string) => {
      if (disposed) return
      setPlayerState(state)
      setMessage(text)
    }

    const play = () => {
      if (disposed) return
      video.play().then(() => {
        update('playing', '')
      }).catch(() => {
        // Autoplay can be blocked by browser policy. The native controls remain available.
        update('waiting', 'Нажмите ▶ на видео, чтобы начать просмотр')
      })
    }

    const retryLater = (reason: string) => {
      if (disposed) return
      if (retryCount >= MAX_RETRIES) {
        update('error', reason + '. Нажмите «Переподключить», чтобы попробовать снова.')
        return
      }
      retryCount += 1
      update('waiting', reason + ' · повтор ' + retryCount + '/' + MAX_RETRIES)
      if (retryTimer) clearTimeout(retryTimer)
      retryTimer = setTimeout(() => {
        if (disposed) return
        if (hls) {
          hls.stopLoad()
          hls.startLoad(-1)
        } else if (nativeMode) {
          video.load()
        }
      }, Math.min(1000 * retryCount, 6000))
    }

    const onPlaying = () => update('playing', '')
    const onWaiting = () => update('waiting', 'Буферизация видеопотока…')
    const onStalled = () => update('waiting', 'Поток временно задерживается…')
    const onNativeError = () => retryLater('Браузер не смог воспроизвести HLS-поток')
    const onNativeMetadata = () => {
      retryCount = 0
      play()
    }

    video.addEventListener('playing', onPlaying)
    video.addEventListener('waiting', onWaiting)
    video.addEventListener('stalled', onStalled)

    const useNative = () => {
      if (disposed) return
      nativeMode = true
      if (hls) {
        hls.destroy()
        hls = null
        hlsRef.current = null
      }
      video.addEventListener('loadedmetadata', onNativeMetadata)
      video.addEventListener('error', onNativeError)
      video.src = source
      video.load()
      update('loading', 'Открываем поток встроенным HLS-плеером…')
    }

    if (Hls.isSupported()) {
      hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        backBufferLength: 20,
        maxBufferLength: 10,
        maxMaxBufferLength: 20,
        liveSyncDurationCount: 3,
        liveMaxLatencyDurationCount: 7,
        capLevelToPlayerSize: true,
        startLevel: -1,
        manifestLoadingMaxRetry: 6,
        manifestLoadingRetryDelay: 800,
        manifestLoadingMaxRetryTimeout: 10000,
        levelLoadingMaxRetry: 6,
        levelLoadingRetryDelay: 800,
        fragLoadingMaxRetry: 6,
        fragLoadingRetryDelay: 800,
      })
      hlsRef.current = hls

      hls.on(Hls.Events.MEDIA_ATTACHED, () => {
        if (!disposed) update('loading', 'Получаем видеопоток…')
      })

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (disposed || !hls) return
        retryCount = 0
        const hevc = hls.levels.some((level) => /^(hvc1|hev1)/i.test(level.videoCodec || ''))
        // Safari/iOS native HLS is often the only available HEVC decoder path.
        if (hevc && video.canPlayType(HLS_MIME)) {
          useNative()
          return
        }
        update('loading', 'Поток найден, запускаем видео…')
        play()
      })

      hls.on(Hls.Events.FRAG_BUFFERED, () => {
        if (!disposed && video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
          retryCount = 0
        }
      })

      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (disposed || !data.fatal) return
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
          retryLater('Поток временно недоступен')
          return
        }
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          const codec = hls?.levels[hls.currentLevel]?.videoCodec || ''
          const hevc = /^(hvc1|hev1)/i.test(codec)
          if (hevc && video.canPlayType(HLS_MIME)) {
            useNative()
            return
          }
          try {
            hls?.recoverMediaError()
            update('waiting', 'Восстанавливаем видеодекодер…')
          } catch {
            retryLater('Ошибка декодирования видео')
          }
          return
        }
        retryLater('Ошибка HLS-плеера: ' + data.details)
      })

      hls.loadSource(source)
      hls.attachMedia(video)
    } else if (video.canPlayType(HLS_MIME)) {
      useNative()
    } else {
      update('error', 'Этот браузер не поддерживает HLS. Для H.265/HEVC нужен браузер и устройство с поддержкой HEVC.')
    }

    return () => {
      disposed = true
      if (retryTimer) clearTimeout(retryTimer)
      if (hls) hls.destroy()
      if (hlsRef.current === hls) hlsRef.current = null
      video.removeEventListener('playing', onPlaying)
      video.removeEventListener('waiting', onWaiting)
      video.removeEventListener('stalled', onStalled)
      video.removeEventListener('loadedmetadata', onNativeMetadata)
      video.removeEventListener('error', onNativeError)
      video.pause()
      video.removeAttribute('src')
      video.load()
    }
  }, [camera?.id, camera?.hls_url, manualRetry])

  useEffect(() => {
    const element = stageRef.current
    if (!element) return
    const onWheel = (event: WheelEvent) => {
      event.preventDefault()
      setScale((value) => Math.min(5, Math.max(1, value - event.deltaY * 0.0015)))
    }
    element.addEventListener('wheel', onWheel, { passive: false })
    return () => element.removeEventListener('wheel', onWheel)
  }, [])

  const resetZoom = () => setScale(1)

  return (
    <main className="live-page">
      <header className="live-header">
        <Link to="/" className="back">← Назад</Link>
        <h1>{camera?.name || 'Прямой эфир'}</h1>
        {camera && <span className={`badge ${camera.online ? 'online' : 'offline'}`}>{camera.online ? 'Online' : 'Ожидание потока'}</span>}
        <span className="zoom-hint">Колесо мыши — масштаб · двойной клик — сброс</span>
      </header>

      <section className="live-stage" ref={stageRef} onDoubleClick={resetZoom}>
        <video
          ref={videoRef}
          className="live-video"
          style={{ transform: `scale(${scale})` }}
          muted
          autoPlay
          playsInline
          controls
          preload="none"
        />
        {(pageLoading || playerState !== 'playing') && (
          <div className={`live-overlay ${playerState === 'error' ? 'live-error-panel' : ''}`}>
            <span>{pageLoading ? 'Загрузка камеры…' : message}</span>
            {playerState === 'error' && camera && (
              <button className="btn primary" onClick={retry}>Переподключить</button>
            )}
          </div>
        )}
      </section>
    </main>
  )
}
