import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { getCameras, CameraStatus } from '../api'
import './Archive.css'

interface RecordingInfo {
  filename: string
  path: string
  size: number
  mod_time: string
  day: string
}

export default function Archive() {
  const [cameras, setCameras] = useState<CameraStatus[]>([])
  const [selectedId, setSelectedId] = useState<string>('')
  const [recordings, setRecordings] = useState<RecordingInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [startLocal, setStartLocal] = useState('')
  const [endLocal, setEndLocal] = useState('')
  const [exporting, setExporting] = useState(false)

  useEffect(() => {
    getCameras()
      .then((list: CameraStatus[]) => {
        setCameras(list)
        if (list.length && !selectedId) setSelectedId(list[0].id)
      })
      .catch((e) => setError(e.message))
  }, [])

  useEffect(() => {
    if (!selectedId) return
    setLoading(true)
    setError('')
    const token = localStorage.getItem('token')
    fetch(`/api/archive/${selectedId}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then(async (res) => {
        if (!res.ok) throw new Error('Не удалось загрузить записи')
        return res.json()
      })
      .then((data: RecordingInfo[]) => setRecordings(data || []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [selectedId])

  const formatSize = (n: number) => {
    if (n < 1024) return `${n} B`
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
    if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
    return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`
  }

  const handleExport = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedId || !startLocal || !endLocal) return
    const start = new Date(startLocal)
    const end = new Date(endLocal)
    if (end <= start) {
      alert('Конец должен быть позже начала')
      return
    }
    setExporting(true)
    setError('')
    try {
      const token = localStorage.getItem('token')
      const res = await fetch('/api/export', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          camera_id: selectedId,
          start_time: start.toISOString(),
          end_time: end.toISOString(),
        }),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error(err.error || 'Ошибка экспорта')
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `export_${selectedId}_${start.toISOString().slice(0, 19)}.mp4`
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch (err: any) {
      setError(err.message || 'Ошибка экспорта')
    } finally {
      setExporting(false)
    }
  }

  const selectedCam = cameras.find((c) => c.id === selectedId)

  return (
    <div className="archive-page">
      <header className="header">
        <div className="logo">NVR</div>
        <nav>
          <Link to="/" className="nav-btn">Потоки</Link>
          <button className="nav-btn active">Архив</button>
        </nav>
        <div className="header-actions">
          <Link to="/" className="btn">← Назад</Link>
        </div>
      </header>

      <main className="archive-content">
        <aside className="archive-sidebar">
          <h2>Камеры</h2>
          {cameras.length === 0 ? (
            <p className="muted">Нет камер</p>
          ) : (
            <ul className="cam-list">
              {cameras.map((cam) => (
                <li key={cam.id}>
                  <button
                    className={cam.id === selectedId ? 'active' : ''}
                    onClick={() => setSelectedId(cam.id)}
                  >
                    <span className={`dot ${cam.online ? 'online' : ''}`} />
                    {cam.name}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </aside>

        <section className="archive-main">
          <h2>Записи {selectedCam ? `— ${selectedCam.name}` : ''}</h2>
          {error && <div className="error-box">{error}</div>}

          <form className="export-form" onSubmit={handleExport}>
            <h3>Скачать фрагмент (без перекодирования)</h3>
            <div className="export-row">
              <label>
                Начало
                <input type="datetime-local" value={startLocal} onChange={(e) => setStartLocal(e.target.value)} required />
              </label>
              <label>
                Конец
                <input type="datetime-local" value={endLocal} onChange={(e) => setEndLocal(e.target.value)} required />
              </label>
              <button type="submit" className="btn primary" disabled={exporting || !selectedId}>
                {exporting ? 'Вырезаем...' : 'Скачать MP4'}
              </button>
            </div>
            <p className="hint">
              Фрагмент вырезается через <code>ffmpeg -c copy</code> — без перекодирования.
            </p>
          </form>

          {loading ? (
            <p className="muted">Загрузка списка записей...</p>
          ) : recordings.length === 0 ? (
            <p className="muted">Записей пока нет. Включите запись на камере на главной.</p>
          ) : (
            <table className="rec-table">
              <thead>
                <tr>
                  <th>Файл</th>
                  <th>День</th>
                  <th>Размер</th>
                  <th>Изменён</th>
                </tr>
              </thead>
              <tbody>
                {recordings.map((r) => (
                  <tr key={r.filename}>
                    <td className="mono">{r.filename}</td>
                    <td>{r.day || '—'}</td>
                    <td>{formatSize(r.size)}</td>
                    <td>{new Date(r.mod_time).toLocaleString('ru')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      </main>
    </div>
  )
}
