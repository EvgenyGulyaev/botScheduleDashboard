<template>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <div class="mb-8">
      <h2 class="text-3xl font-bold text-gray-900">Dashboard</h2>
      <p class="mt-2 text-lg text-gray-600">Статус и управление ботом</p>
    </div>

    <!-- Статус карточка -->
    <div class="grid grid-cols-1 gap-6 mb-8">
      <div class="bg-white p-8 rounded-xl shadow-lg">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-medium text-gray-500 uppercase tracking-wide">Статус бота</p>
            <p class="mt-1 text-4xl font-bold text-gray-900">{{ botStatus }}</p>
          </div>
          <div
            :class="statusClass"
            class="w-20 h-20 rounded-2xl flex items-center justify-center text-3xl"
          >
            {{ statusIcon }}
          </div>
        </div>
        <div class="mt-6">
          <button
            @click="restartBot"
            :disabled="loading"
            class="inline-flex items-center px-6 py-3 bg-yellow-500 text-white text-sm font-medium rounded-lg hover:bg-yellow-600 disabled:opacity-50 transition-colors"
          >
            <span v-if="!loading">🔄 Перезапустить</span>
            <span v-else>⏳ Перезапуск...</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useAuthStore } from '../stores/auth.js'

const authStore = useAuthStore()
const botStatus = ref('unknown')
const loading = ref(false)

const statusClass = computed(() =>
  botStatus.value === 'active'
    ? 'bg-green-100 border-4 border-green-400'
    : botStatus.value === 'error'
      ? 'bg-red-100 border-4 border-red-400'
      : 'bg-yellow-100 border-4 border-yellow-400',
)

const statusIcon = computed(() =>
  botStatus.value === 'active' ? '🟢' : botStatus.value === 'error' ? '🔴' : '🟡',
)

const loadStatus = async () => {
  try {
    const res = await authStore.api.get('/bot/status')
    botStatus.value = res.data.status || 'unknown'
  } catch {
    botStatus.value = 'error'
  }
}

const restartBot = async () => {
  loading.value = true
  try {
    await authStore.api.post('/bot/restart', {})
    alert('✅ Бот перезапущен!')
    loadStatus()
  } catch (error) {
    alert('❌ ' + (error.response?.data?.message || 'Ошибка'))
  } finally {
    loading.value = false
  }
}

onMounted(loadStatus)
</script>
