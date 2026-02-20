<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-4">
      <router-link to="/library" class="text-gray-400 hover:text-white">
        ← Back to Library
      </router-link>
      <button
        v-if="item"
        @click="confirmDelete"
        class="text-red-400 hover:text-red-300"
      >
        Delete
      </button>
    </header>

    <main v-if="loading" class="text-center text-gray-400 py-8">
      Loading...
    </main>

    <main v-else-if="error" class="text-center text-red-400 py-8">
      {{ error }}
    </main>

    <main v-else-if="item" class="max-w-4xl mx-auto">
      <div class="bg-[#0a2540] rounded-lg overflow-hidden">
        <div class="aspect-video bg-black flex items-center justify-center relative">
          <video
            v-if="item.type === 'video' && (item.master_url || item.media_url)"
            controls
            playsinline
            class="max-h-full"
          >
            <source :src="item.master_url || item.media_url" />
            Your browser does not support video playback.
          </video>
          <div
            v-else-if="item.type === 'video' && !item.master_url && !item.media_url"
            class="absolute inset-0 flex flex-col items-center justify-center bg-black/80"
          >
            <span class="text-4xl mb-2">⏳</span>
            <span class="text-gray-300">Processing...</span>
            <span v-if="item.processing_status" class="text-gray-400 text-sm mt-1">
              Status: {{ item.processing_status }}
            </span>
          </div>
          <img
            v-else-if="item.type === 'photo' && item.media_url"
            :src="item.media_url"
            :alt="item.title"
            class="max-h-full max-w-full object-contain"
          />
          <span v-else class="text-6xl">
            {{ item.type === 'video' ? '🎬' : '📷' }}
          </span>
        </div>

        <div class="p-6">
          <div class="flex items-center gap-3 mb-2">
            <h1 class="text-2xl font-bold text-white">
              {{ item.title || 'Untitled' }}
            </h1>
            <span
              v-if="item.type === 'video' && item.processing_status"
              :class="[
                'px-2 py-1 rounded text-xs font-medium',
                item.processing_status === 'ready' ? 'bg-green-600 text-white' :
                item.processing_status === 'processing' ? 'bg-yellow-600 text-white' :
                item.processing_status === 'failed' ? 'bg-red-600 text-white' :
                'bg-gray-600 text-gray-300'
              ]"
            >
              {{ item.processing_status }}
            </span>
          </div>
          <div class="flex items-center gap-4 mb-4">
            <PersonaBadge :display-name="getPersonaDisplayName(item)" />
            <p class="text-gray-400">
              {{ formatDate(item.created_at) }}
            </p>
          </div>
          <p v-if="item.description" class="text-gray-300">
            {{ item.description }}
          </p>
          <p v-if="item.error_message" class="text-red-400 mt-4">
            Error: {{ item.error_message }}
          </p>
        </div>
      </div>

      <div v-if="item.type === 'video' && item.processing_status === 'ready'" class="mt-6 bg-[#0a2540] rounded-lg p-6">
        <h2 class="text-xl font-bold text-white mb-4">Create Short</h2>
        
        <div class="flex gap-4 mb-4">
          <div class="flex-1">
            <label class="block text-gray-400 text-sm mb-1">Start (seconds)</label>
            <input
              v-model.number="clipStart"
              type="number"
              min="0"
              :max="clipEnd - 1"
              class="w-full bg-gray-800 text-white px-3 py-2 rounded-lg"
            />
          </div>
          <div class="flex-1">
            <label class="block text-gray-400 text-sm mb-1">End (seconds)</label>
            <input
              v-model.number="clipEnd"
              type="number"
              :min="clipStart + 1"
              class="w-full bg-gray-800 text-white px-3 py-2 rounded-lg"
            />
          </div>
        </div>
        
        <p v-if="clipError" class="text-red-400 text-sm mb-4">{{ clipError }}</p>
        
        <div class="flex gap-4">
          <button
            @click="createNewClip"
            :disabled="clipCreating"
            class="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
          >
            {{ clipCreating ? 'Creating...' : 'Create Short' }}
          </button>
          <router-link
            v-if="clips.length > 0"
            to="/shorts"
            class="text-blue-400 hover:text-blue-300 px-4 py-2"
          >
            View Shorts →
          </router-link>
        </div>
      </div>

      <div v-if="clips.length > 0" class="mt-6 bg-[#0a2540] rounded-lg p-6">
        <h2 class="text-xl font-bold text-white mb-4">Shorts from this video</h2>
        
        <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
          <div
            v-for="clip in clips"
            :key="clip.clip_id"
            class="bg-gray-800 rounded-lg overflow-hidden"
          >
            <div class="aspect-[9/16] bg-black flex items-center justify-center relative">
              <video
                v-if="clip.status === 'ready'"
                :src="clip.video_url"
                class="w-full h-full object-cover"
                muted
                @mouseenter="$event.target.play()"
                @mouseleave="$event.target.pause()"
              ></video>
              <div v-else class="flex flex-col items-center text-gray-400">
                <div class="w-6 h-6 border-2 border-gray-400 border-t-transparent rounded-full animate-spin mb-2"></div>
                <span class="text-sm">Processing</span>
              </div>
            </div>
            <div class="p-2">
              <p class="text-gray-400 text-xs">{{ formatDuration(clip.duration_ms) }}</p>
            </div>
          </div>
        </div>
      </div>

      <div v-if="showDeleteConfirm" class="fixed inset-0 bg-black/50 flex items-center justify-center p-4">
        <div class="bg-[#0a2540] rounded-lg p-6 max-w-sm w-full">
          <h3 class="text-lg font-semibold text-white mb-4">Delete this item?</h3>
          <p class="text-gray-400 mb-6">This action cannot be undone.</p>
          <div class="flex gap-4">
            <button
              @click="showDeleteConfirm = false"
              class="flex-1 bg-gray-700 hover:bg-gray-600 text-white py-2 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              @click="doDelete"
              class="flex-1 bg-red-600 hover:bg-red-700 text-white py-2 rounded-lg transition-colors"
            >
              Delete
            </button>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getItem, deleteItem, createClip, getItemClips, getPersonaDisplayName } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const route = useRoute()
const router = useRouter()

const item = ref(null)
const loading = ref(true)
const error = ref('')
const showDeleteConfirm = ref(false)
const clips = ref([])
const clipStart = ref(0)
const clipEnd = ref(10)
const clipCreating = ref(false)
const clipError = ref('')

async function loadItem() {
  loading.value = true
  error.value = ''
  
  try {
    item.value = await getItem(route.params.id)
    if (item.value.type === 'video') {
      await loadClips()
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function loadClips() {
  try {
    const result = await getItemClips(route.params.id)
    clips.value = result.clips || []
  } catch (e) {
    console.error('Failed to load clips:', e)
  }
}

function formatDate(dateStr) {
  const date = new Date(dateStr)
  return date.toLocaleString()
}

function formatDuration(ms) {
  const secs = Math.floor(ms / 1000)
  const mins = Math.floor(secs / 60)
  const remainingSecs = secs % 60
  return `${mins}:${remainingSecs.toString().padStart(2, '0')}`
}

function confirmDelete() {
  showDeleteConfirm.value = true
}

async function doDelete() {
  try {
    await deleteItem(route.params.id)
    router.push('/library')
  } catch (e) {
    error.value = e.message
    showDeleteConfirm.value = false
  }
}

async function createNewClip() {
  clipError.value = ''
  
  const startMs = Math.floor(clipStart.value * 1000)
  const endMs = Math.floor(clipEnd.value * 1000)
  
  if (endMs - startMs > 90000) {
    clipError.value = 'Clip cannot exceed 90 seconds'
    return
  }
  
  if (endMs <= startMs) {
    clipError.value = 'End time must be greater than start time'
    return
  }
  
  clipCreating.value = true
  
  try {
    await createClip(route.params.id, startMs, endMs)
    clipStart.value = 0
    clipEnd.value = 10
    await loadClips()
  } catch (e) {
    clipError.value = e.message
  } finally {
    clipCreating.value = false
  }
}

onMounted(loadItem)
</script>
