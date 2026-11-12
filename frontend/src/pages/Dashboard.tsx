import { useEffect, useState, useCallback, useRef } from 'react'
import { Link } from 'react-router-dom'
import { getCameras, createCamera, deleteCamera, startRecording, stopRecording, CameraStatus } from '../api'
import './Dashboard.css'

interface Props {
  onLogout: () => void
}

export default function Dashboard({ onLogout }: Props) {
  const [cameras, setCameras] = useState<CameraStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [busyCameraId, setBusyCameraId] = useState('')
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: '', rtsp_url: '', username: '', password: '' })
  const loadInFlight = useRef(false)

  const load = useCallback(async () => {
    if (loadInFlight.current) return
    loadInFlight.current = true
    try {
      const data = await getCameras()
      setCameras(data)
      setLoadError('')
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : 'Не удалось загрузить камеры')
    } finally {
      setLoading(false)
      loadInFlight.current = false
    }
  }, [])

  // Schedule the next refresh after the current request finishes. This avoids
  // overlapping API calls when the page is slow or the camera list is large.
  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>
    const refresh = async () => {
      await load()
      if (!cancelled) timer = setTimeout(refresh, 15000)
    }
    void refresh()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [load])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setFormError('')
    const name = form.name.trim()
    const rtspUrl = form.rtsp_url.trim()
    let parsed: URL
    try {
      parsed = new URL(rtspUrl)
    } catch {
      setFormError('Укажите полный RTSP-адрес, например rtsp://192.168.1.100:554/stream')
      return
    }
    if (!name || !['rtsp:', 'rtsps:'].includes(parsed.protocol) || !parsed.hostname) {
      setFormError('Проверьте название и RTSP-адрес камеры')
      return
    }
    setSubmitting(true)
    try {
      await createCamera({ ...form, name, rtsp_url: rtspUrl, username: form.username.trim() })
      setShowAdd(false)
      setForm({ name: '', rtsp_url: '', username: '', password: '' })
      await load()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Ошибка добавления камеры')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Удалить камеру «${name}»?`)) return
    try {
      await deleteCamera(id)
      await load()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Ошибка удаления камеры')
    }
  }

  const toggleRecord = async (cam: CameraStatus) => {
    if (busyCameraId) return
    setBusyCameraId(cam.id)
    try {
      if (cam.recording) await stopRecording(cam.id)
      else await startRecording(cam.id)
      await load()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Не удалось изменить состояние записи')
    } finally {
      setBusyCameraId('')
    }
  }

  return (
    <div className="dashboard">
      <header className="header">
        <div className="logo">NVR</div>
        <nav>
          <button className="nav-btn active">Потоки</button>
          <Link to="/archive" className="nav-btn">Архив</Link>
        </nav>
        <div className="header-actions">
          <button className="btn primary" onClick={() => setShowAdd(true)}>+ Добавить поток</button>
          <button className="btn" onClick={onLogout}>Выйти</button>
        </div>
      </header>

      <main className="content">
        {loadError && <div className="form-error">{loadError} <button className="btn" onClick={load}>Повторить</button></div>}
        {loading && cameras.length === 0 ? (
          <div className="empty">Загрузка...</div>
        ) : cameras.length === 0 ? (
          <div className="empty">
            <p>Нет камер</p>
            <button className="btn primary" onClick={() => setShowAdd(true)}>Добавить первую камеру</button>
          </div>
        ) : (
          <div className="grid">
            {cameras.map((cam) => (
              <div key={cam.id} className="card">
                <Link to={`/live/${cam.id}`} className="preview-link">
                  <div className="preview">
                    {cam.online && cam.preview_url ? (
                      <img
                        src={`${cam.preview_url}?t=${encodeURIComponent(cam.last_frame_at || '')}`}
                        alt={cam.name}
                        loading="lazy"
                        decoding="async"
                      />
                    ) : (
                      <div className="no-preview">Превью недоступно</div>
                    )}
                    <div className="status-dot" data-online={cam.online} />
                    {cam.recording && <div className="rec-badge">REC</div>}
                    {cam.last_frame_at && (
                      <div className="timestamp">{new Date(cam.last_frame_at).toLocaleString('ru')}</div>
                    )}
                  </div>
                </Link>
                <div className="card-footer">
                  <span className="name">{cam.name}</span>
                  <div className="card-actions">
                    <button
                      className="icon-btn"
                      title={cam.recording ? 'Остановить запись' : 'Начать запись'}
                      disabled={busyCameraId === cam.id}
                      onClick={() => toggleRecord(cam)}
                    >
                      {busyCameraId === cam.id ? '…' : cam.recording ? '⏹' : '⏺'}
                    </button>
                    <button className="icon-btn danger" title="Удалить" onClick={() => handleDelete(cam.id, cam.name)}>×</button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </main>

      {showAdd && (
        <div className="modal-overlay" onClick={() => !submitting && setShowAdd(false)}>
          <form className="modal" onClick={(e) => e.stopPropagation()} onSubmit={handleCreate}>
            <h2>Добавить поток</h2>
            <label>Название<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Камера 1" /></label>
            <label>RTSP URL<input required autoComplete="off" value={form.rtsp_url} onChange={(e) => setForm({ ...form, rtsp_url: e.target.value })} placeholder="rtsp://192.168.1.100:554/stream" /></label>
            <p className="form-help">Укажите полный адрес потока. Если камера требует авторизацию, заполните логин и пароль ниже.</p>
            <label>Логин камеры<input autoComplete="off" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></label>
            <label>Пароль камеры<input type="password" autoComplete="new-password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} /></label>
            {formError && <div className="form-error">{formError}</div>}
            <div className="modal-actions">
              <button type="button" className="btn" disabled={submitting} onClick={() => setShowAdd(false)}>Отмена</button>
              <button type="submit" className="btn primary" disabled={submitting}>{submitting ? 'Сохраняем…' : 'Добавить камеру'}</button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}
