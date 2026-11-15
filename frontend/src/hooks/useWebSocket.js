import { useEffect, useEffectEvent, useState } from 'react'
import { useSelector } from 'react-redux'
import socketService from '@/services/socketService'
import { selectToken } from '@/redux/slices/authSlice'

/**
 * Connects to the realtime server while the user is logged in.
 * `onEvent` receives every server event, e.g. { type: 'message:new', data: message }.
 */
export default function useWebSocket(onEvent) {
  const token = useSelector(selectToken)
  const [connected, setConnected] = useState(() => socketService.isConnected())

  const handleEvent = useEffectEvent((event) => {
    if (event.type === 'socket:open') setConnected(true)
    else if (event.type === 'socket:close') setConnected(false)
    else onEvent?.(event)
  })

  useEffect(() => socketService.subscribe((event) => handleEvent(event)), [])

  useEffect(() => {
    if (token) socketService.connect(token)
    else socketService.disconnect()
  }, [token])

  return {
    connected: Boolean(token) && connected,
    send: (payload) => socketService.send(payload),
  }
}
