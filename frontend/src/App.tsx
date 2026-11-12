import { Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Live from './pages/Live'
import Archive from './pages/Archive'

function App() {
  const [token, setToken] = useState<string | null>(localStorage.getItem('token'))

  useEffect(() => {
    if (token) localStorage.setItem('token', token)
    else localStorage.removeItem('token')
  }, [token])

  const logout = () => setToken(null)

  return (
    <Routes>
      <Route path="/login" element={<Login onLogin={setToken} />} />
      <Route
        path="/"
        element={token ? <Dashboard onLogout={logout} /> : <Navigate to="/login" />}
      />
      <Route
        path="/live/:id"
        element={token ? <Live /> : <Navigate to="/login" />}
      />
      <Route
        path="/archive"
        element={token ? <Archive /> : <Navigate to="/login" />}
      />
    </Routes>
  )
}

export default App
