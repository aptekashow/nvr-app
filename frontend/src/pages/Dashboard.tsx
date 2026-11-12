import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { getCameras, createCamera, deleteCamera, startRecording, stopRecording, CameraStatus } from '../api'
import './Dashboard.css'

interface Props {
  onLogout: () => void
}

export default function Dashboard({ onLogout }: Props) {
  const [cameras, setCameras] = useState<CameraStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: '', rtsp_url: '', username: '', password: '' })

  const load = useCallback(async () => {
    try {
      const data = await getCameras()
      setCameras(data)
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [load])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await createCamera(form)
      setShowAdd(false)
      setForm({ name: '', rtsp_url: '', username: '', password: '' })
      load()
    } catch (err: any) {
      alert(err.message)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Удалить камеру «${name}»?`)) return
    await deleteCamera(id)
    load()
  }

  const toggleRecord = async (cam: CameraStatus) => {
    try {
      if (cam.recording) await stopRecording(cam.id)
      else await startRecording(cam.id)
      load()
    } catch (err: any) {
      alert(err.message)
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
                      <img src={`${cam.preview_url}?t=${Date.now()}`} alt={cam.name} loading="lazy" />
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
                    <button className="icon-btn" title={cam.recording ? 'Остановить запись' : 'Начать запись'} onClick={() => toggleRecord(cam)}>
                      {cam.recording ? '⏹' : '⏺'}
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
        <div className="modal-overlay" onClick={() => setShowAdd(false)}>
          <form className="modal" onClick={(e) => e.stopPropagation()} onSubmit={handleCreate}>
            <h2>Добавить поток</h2>
            <label>Название<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Камера 1" /></label>
            <label>RTSP URL<input required value={form.rtsp_url} onChange={(e) => setForm({ ...form, rtsp_url: e.target.value })} placeholder="rtsp://user:pass@192.168.1.100:554/stream" /></label>
            <label>Логин камеры<input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} /></label>
            <label>Пароль камеры<input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} /></label>
            <div className="modal-actions">
              <button type="button" className="btn" onClick={() => setShowAdd(false)}>Отмена</button>
              <button type="submit" className="btn primary">Добавить</button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}
