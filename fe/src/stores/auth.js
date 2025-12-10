import { defineStore } from 'pinia'
import axios from 'axios'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || null,
    api: axios.create({
      baseURL: 'http://localhost:8080',
      headers: { 'Content-Type': 'application/json' },
    }),
  }),

  actions: {
    async login(credentials) {
      const res = await this.api.post('/login', credentials)
      this.token = res.data.Token
      localStorage.setItem('token', this.token)
      this.api.defaults.headers.common['Authorization'] = `Bearer ${this.token}`
    },

    logout() {
      this.token = null
      localStorage.removeItem('token')
      delete this.api.defaults.headers.common['Authorization']
    },
  },

  getters: {
    isAuthenticated: (state) => !!state.token,
  },
})
