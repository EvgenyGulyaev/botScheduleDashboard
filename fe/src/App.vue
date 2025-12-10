<template>
  <div class="min-h-screen bg-gray-50">
    <!-- Навигация (только после логина) -->
    <nav v-if="$route.meta.requiresAuth" class="bg-white shadow-sm border-b">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div class="flex justify-between items-center h-16">
          <div class="flex items-center space-x-4">
            <h1 class="text-xl font-bold text-gray-900">🤖 Bot Admin</h1>
          </div>
          <div class="flex items-center space-x-4">
            <router-link
              to="/dashboard"
              class="px-4 py-2 text-sm font-medium rounded-md hover:bg-gray-100"
              >📊 Dashboard</router-link
            >
            <router-link
              to="/messages"
              class="px-4 py-2 text-sm font-medium rounded-md hover:bg-gray-100"
              >💬 Сообщения</router-link
            >
            <router-link
              to="/history"
              class="px-4 py-2 text-sm font-medium rounded-md hover:bg-gray-100"
              >📋 История</router-link
            >
            <button
              @click="logout"
              class="px-4 py-2 bg-red-500 text-white text-sm rounded-md hover:bg-red-600"
            >
              🚪 Выход
            </button>
          </div>
        </div>
      </div>
    </nav>

    <!-- Контент -->
    <div class="pt-4 pb-12">
      <router-view />
    </div>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { useAuthStore } from './stores/auth.js'

const router = useRouter()
const authStore = useAuthStore()

const logout = () => {
  authStore.logout()
  router.push('/login')
}
</script>
