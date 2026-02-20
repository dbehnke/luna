<template>
  <div class="min-h-screen bg-[#07182c] flex flex-col">
    <!-- Top bar with logo icon -->
    <header class="p-4">
      <img
        :src="lunaMark"
        alt="Luna"
        class="w-12 h-12"
      />
    </header>

    <!-- Main content -->
    <main class="flex-1 flex flex-col items-center justify-center px-4">
      <!-- Full logo -->
      <img
        :src="lunaLogo"
        alt="Luna"
        class="w-full max-w-md mb-8"
      />

      <!-- App name -->
      <h1 class="text-4xl font-bold text-white mb-2">Luna</h1>

      <!-- Tagline -->
      <p class="text-xl text-gray-400 mb-8">Moments live here</p>

      <!-- Navigation buttons -->
      <div class="flex gap-4">
        <router-link
          to="/shorts"
          class="px-6 py-3 bg-purple-600 hover:bg-purple-700 text-white font-medium rounded-lg transition-colors"
        >
          Shorts
        </router-link>
        <router-link
          to="/library"
          class="px-6 py-3 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-lg transition-colors"
        >
          Library
        </router-link>
        <router-link
          to="/upload"
          class="px-6 py-3 bg-gray-700 hover:bg-gray-600 text-white font-medium rounded-lg transition-colors"
        >
          Upload
        </router-link>
        <router-link
          to="/me/profile"
          class="px-6 py-3 bg-gray-700 hover:bg-gray-600 text-white font-medium rounded-lg transition-colors"
        >
          Profile
        </router-link>
        <router-link
          v-if="isAdmin"
          to="/admin/users"
          class="px-6 py-3 bg-emerald-700 hover:bg-emerald-600 text-white font-medium rounded-lg transition-colors"
        >
          Admin
        </router-link>
      </div>
    </main>

    <!-- Footer -->
    <footer class="p-4 text-center text-gray-500 text-sm">
      <p>Family media server</p>
    </footer>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import lunaMark from '/src/assets/brand/luna-mark.png'
import lunaLogo from '/src/assets/brand/luna-logo-full.png'
import { getCurrentUser } from '../services/api'

const isAdmin = ref(false)

onMounted(async () => {
  const user = await getCurrentUser()
  isAdmin.value = user?.role === 'admin'
})
</script>
