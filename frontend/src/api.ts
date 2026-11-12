const API = '/api'

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export async function login(username: string, password: string) {
  const res = await fetch(`${API}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) throw new Error('Неверный логин или пароль')
  return res.json()
}

export async function getCameras() {
  const res = await fetch(`${API}/cameras`, {
    headers: authHeaders(),
  })
  if (!res.ok) throw new Error('Ошибка загрузки камер')
  return res.json()
}

export async function createCamera(data: {
  name: string
  rtsp_url: string
  username?: string
  password?: string
}) {
  const res = await fetch(`${API}/cameras`, {
    method: 'POST',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error || 'Ошибка создания')
  }
  return res.json()
}

export async function deleteCamera(id: string) {
  const res = await fetch(`${API}/cameras/${id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  })
  if (!res.ok) throw new Error('Ошибка удаления')
}

export async function startRecording(id: string) {
  const res = await fetch(`${API}/cameras/${id}/record/start`, {
    method: 'POST',
    headers: authHeaders(),
  })
  if (!res.ok) throw new Error('Не удалось начать запись')
}

export async function stopRecording(id: string) {
  const res = await fetch(`${API}/cameras/${id}/record/stop`, {
    method: 'POST',
    headers: authHeaders(),
  })
  if (!res.ok) throw new Error('Не удалось остановить запись')
}

export interface CameraStatus {
  id: string
  name: string
  online: boolean
  recording: boolean
  preview_url: string
  last_frame_at: string
  hls_url: string
}
