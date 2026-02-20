<template>
  <div class="min-h-screen bg-[#07182c] flex items-center justify-center p-4">
    <div class="w-full max-w-md bg-[#0a2540] rounded-lg p-6 space-y-4">
      <h1 class="text-2xl font-bold text-white">Sign In</h1>
      <p class="text-gray-400 text-sm">Use your Luna account to continue.</p>

      <form class="space-y-4" @submit.prevent="submit">
        <div>
          <label class="block text-sm text-gray-300 mb-1">Username</label>
          <input
            v-model="username"
            type="text"
            autocomplete="username"
            class="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-blue-500"
            required
          />
        </div>

        <div>
          <label class="block text-sm text-gray-300 mb-1">Password</label>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
            class="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-blue-500"
            required
          />
        </div>

        <button
          type="submit"
          :disabled="submitting"
          class="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white font-medium py-2 rounded-lg transition-colors"
        >
          {{ submitting ? 'Signing in...' : 'Sign In' }}
        </button>
      </form>

      <p v-if="error" class="text-red-400 text-sm">{{ error }}</p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../services/api'

const router = useRouter()
const route = useRoute()

const username = ref('')
const password = ref('')
const submitting = ref(false)
const error = ref('')

async function submit() {
  submitting.value = true
  error.value = ''

  try {
    await login(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    router.push(redirect)
  } catch (e) {
    error.value = e.message || 'Login failed'
  } finally {
    submitting.value = false
  }
}
</script>
