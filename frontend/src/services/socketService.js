// Thin WebSocket wrapper with auto-reconnect and a simple pub/sub API.
// Server pushes JSON frames like: { type: "message:new", data: message }
// Local status events are also emitted: { type: "socket:open" } / { type: "socket:close" }

const MAX_RECONNECT_DELAY = 30_000

function resolveBaseUrl() {
  const fromEnv = import.meta.env.VITE_WS_URL
  if (fromEnv) return fromEnv.replace(/\/+$/, '')
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}`
}

class SocketService {
  socket = null
  token = null
  listeners = new Set()
  reconnectAttempts = 0
  reconnectTimer = null
  shouldReconnect = false

  /** Open a connection for `token` (no-op if already connected with the same token). */
  connect(token) {
    if (!token) return
    const active =
      this.socket &&
      (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING)
    if (token === this.token && active) return

    this.disconnect()
    this.token = token
    this.shouldReconnect = true
    this.open()
  }

  open() {
    const url = `${resolveBaseUrl()}/ws?token=${encodeURIComponent(this.token)}`
    const socket = new WebSocket(url)
    this.socket = socket

    socket.onopen = () => {
      this.reconnectAttempts = 0
      this.emit({ type: 'socket:open' })
    }

    socket.onmessage = (event) => {
      try {
        this.emit(JSON.parse(event.data))
      } catch {
        this.emit({ type: 'socket:raw', data: event.data })
      }
    }

    socket.onclose = () => {
      if (this.socket !== socket) return
      this.socket = null
      this.emit({ type: 'socket:close' })
      this.scheduleReconnect()
    }
  }

  scheduleReconnect() {
    if (!this.shouldReconnect || !this.token) return
    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, MAX_RECONNECT_DELAY)
    this.reconnectAttempts += 1
    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = setTimeout(() => this.open(), delay)
  }

  disconnect() {
    this.shouldReconnect = false
    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = null
    this.reconnectAttempts = 0
    this.token = null
    if (this.socket) {
      const socket = this.socket
      this.socket = null
      socket.close()
    }
  }

  send(payload) {
    if (this.socket?.readyState !== WebSocket.OPEN) return false
    this.socket.send(typeof payload === 'string' ? payload : JSON.stringify(payload))
    return true
  }

  isConnected() {
    return this.socket?.readyState === WebSocket.OPEN
  }

  /** Register a listener; returns an unsubscribe function. */
  subscribe(listener) {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  emit(event) {
    this.listeners.forEach((listener) => listener(event))
  }
}

const socketService = new SocketService()
export default socketService
