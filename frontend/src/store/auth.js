import { defineStore } from 'pinia'
import { api, getToken, setToken } from '../api'

export const useAuth = defineStore('auth', {
  state: () => ({
    user: null,
    ready: false,
  }),
  getters: {
    signedIn: (s) => !!s.user,
    initial: (s) => (s.user?.displayName || s.user?.email || '?').trim().charAt(0).toUpperCase(),
  },
  actions: {
    // Resolve the stored token once at start-up. The app renders the moment
    // this settles either way, so a guest is never held at a splash screen.
    async restore() {
      if (!getToken()) {
        this.ready = true
        return
      }
      try {
        const r = await api.me()
        this.user = r.user
      } catch {
        setToken('')
        this.user = null
      } finally {
        this.ready = true
      }
    },
    accept({ token, user }) {
      setToken(token)
      this.user = user
    },
    async signOut() {
      try {
        await api.logout()
      } catch {
        /* the local token is cleared regardless */
      }
      setToken('')
      this.user = null
    },
  },
})
