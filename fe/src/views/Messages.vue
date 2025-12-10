<template>
  <div class="max-w-2xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <div class="mb-8">
      <h2 class="text-3xl font-bold text-gray-900">Отправка сообщений</h2>
      <p class="mt-2 text-lg text-gray-600">Быстрая отправка в Telegram</p>
    </div>

    <form @submit.prevent="sendMessage" class="bg-white p-8 rounded-2xl shadow-xl">
      <div class="space-y-6">
        <div>
          <label class="block text-sm font-semibold text-gray-700 mb-3">Chat ID</label>
          <input
            v-model="form.chat_id"
            required
            placeholder="-100123456789"
            class="w-full px-4 py-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          />
        </div>
        <div>
          <label class="block text-sm font-semibold text-gray-700 mb-3">Сообщение</label>
          <textarea
            v-model="form.text"
            rows="5"
            required
            placeholder="Введите текст сообщения..."
            class="w-full px-4 py-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-blue-500 focus:border-transparent resize-vertical"
          />
        </div>
        <button
          type="submit"
          :disabled="loading"
          class="w-full bg-gradient-to-r from-green-500 to-green-600 text-white py-4 px-6 rounded-xl hover:from-green-600 hover:to-green-700 disabled:opacity-50 font-semibold text-lg shadow-lg"
        >
          📤 {{ loading ? 'Отправляем...' : 'Отправить сообщение' }}
        </button>
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth.js'

const authStore = useAuthStore()
const form = ref({ chat_id: '', text: '' })
const loading = ref(false)

const sendMessage = async () => {
  loading.value = true
  try {
    await authStore.api.post('/message/send', form.value)
    alert('✅ Сообщение отправлено!')
    form.value = { chat_id: '', text: '' }
  } catch (error) {
    alert('❌ ' + (error.response?.data?.message || 'Ошибка отправки'))
  } finally {
    loading.value = false
  }
}
</script>
