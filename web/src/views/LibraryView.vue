<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-8">
      <router-link to="/" class="text-gray-400 hover:text-white">
        ← Back
      </router-link>
      <h1 class="text-xl font-bold text-white">Library</h1>
      <router-link
        to="/upload"
        class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
      >
        + Upload
      </router-link>
    </header>

    <main>
      <div class="flex gap-2 mb-6">
        <button
          @click="filterType = ''"
          :class="[
            'px-4 py-2 rounded-lg transition-colors',
            filterType === '' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400'
          ]"
        >
          All
        </button>
        <button
          @click="filterType = 'video'"
          :class="[
            'px-4 py-2 rounded-lg transition-colors',
            filterType === 'video' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400'
          ]"
        >
          Videos
        </button>
        <button
          @click="filterType = 'photo'"
          :class="[
            'px-4 py-2 rounded-lg transition-colors',
            filterType === 'photo' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400'
          ]"
        >
          Photos
        </button>
        <button
          @click="filterType = 'audio'"
          :class="[
            'px-4 py-2 rounded-lg transition-colors',
            filterType === 'audio' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400'
          ]"
        >
          Audio
        </button>
      </div>

      <div v-if="loading" class="text-center text-gray-400 py-8">
        Loading...
      </div>

      <div v-else-if="error" class="text-center text-red-400 py-8">
        {{ error }}
      </div>

      <div v-else-if="items.length === 0" class="text-center text-gray-400 py-8">
        <p class="text-lg mb-4">No media yet</p>
        <router-link to="/upload" class="text-blue-400 hover:text-blue-300">
          Upload your first item →
        </router-link>
      </div>

      <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        <router-link
          v-for="item in items"
          :key="item.id"
          :to="`/item/${item.id}`"
          class="bg-[#0a2540] rounded-lg overflow-hidden hover:ring-2 hover:ring-blue-500 transition-all"
        >
          <div class="aspect-video bg-gray-800 flex items-center justify-center relative">
            <img
              v-if="item.type === 'photo' && item.media_url"
              :src="item.media_url"
              :alt="item.title"
              class="w-full h-full object-cover"
            />
            <img
              v-else-if="item.type === 'video' && item.thumb_urls && item.thumb_urls.length > 0"
              :src="item.thumb_urls[0]"
              :alt="item.title"
              class="w-full h-full object-cover"
            />
            <span v-else-if="item.type === 'audio'" class="text-4xl">🎵</span>
            <span v-else class="text-4xl">
              {{ item.type === 'video' ? '🎬' : '📷' }}
            </span>
            <span
              v-if="(item.type === 'video' || item.type === 'audio') && item.processing_status && item.processing_status !== 'ready'"
              class="absolute top-2 right-2 px-2 py-1 rounded text-xs font-medium bg-black/70 text-white"
            >
              {{ item.processing_status }}
            </span>
          </div>
          <div class="p-3">
            <div class="flex items-center gap-2 mb-1">
              <PersonaBadge :display-name="getPersonaDisplayName(item)" :avatar-url="getPersonaAvatarUrl(item)" :slug="getPersonaSlug(item)" variant="compact" />
            </div>
            <h3 class="text-white font-medium truncate">{{ item.title || 'Untitled' }}</h3>
            <p class="text-gray-400 text-sm">{{ formatDate(item.created_at) }}</p>
          </div>
        </router-link>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { listItems, getPersonaDisplayName, getPersonaAvatarUrl, getPersonaSlug } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const items = ref([])
const loading = ref(true)
const error = ref('')
const filterType = ref('')

async function loadItems() {
  loading.value = true
  error.value = ''
  
  try {
    const options = {}
    if (filterType.value) {
      options.type = filterType.value
    }
    const result = await listItems(options)
    items.value = result.items
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function formatDate(dateStr) {
  const date = new Date(dateStr)
  return date.toLocaleDateString()
}

watch(filterType, loadItems)

onMounted(loadItems)
</script>
