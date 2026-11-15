// Safe wrapper around localStorage: never throws (private mode, blocked storage, SSR...).
export const STORAGE_KEYS = {
  TOKEN: 'vuathau.token',
  THEME: 'vuathau.theme',
}

function getStorage() {
  try {
    return typeof window !== 'undefined' ? window.localStorage : null
  } catch {
    return null
  }
}

const storageService = {
  get(key, fallback = null) {
    try {
      const raw = getStorage()?.getItem(key)
      return raw == null ? fallback : JSON.parse(raw)
    } catch {
      return fallback
    }
  },

  set(key, value) {
    try {
      getStorage()?.setItem(key, JSON.stringify(value))
    } catch {
      // ignore quota / access errors
    }
  },

  remove(key) {
    try {
      getStorage()?.removeItem(key)
    } catch {
      // ignore
    }
  },

  getToken() {
    return this.get(STORAGE_KEYS.TOKEN)
  },

  setToken(token) {
    if (token) this.set(STORAGE_KEYS.TOKEN, token)
    else this.remove(STORAGE_KEYS.TOKEN)
  },

  clearToken() {
    this.remove(STORAGE_KEYS.TOKEN)
  },
}

export default storageService
