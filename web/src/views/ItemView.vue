<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-4">
      <router-link to="/library" class="text-gray-400 hover:text-white">
        ← Back to Library
      </router-link>
      <div v-if="item && canManageItem" class="flex items-center gap-3">
        <button
          @click="startEdit"
          class="text-blue-300 hover:text-blue-200 text-sm"
        >
          Edit
        </button>
        <button
          @click="triggerReprocess"
          class="text-amber-300 hover:text-amber-200 text-sm"
        >
          Reprocess
        </button>
        <button
          @click="confirmDelete"
          class="text-red-400 hover:text-red-300 text-sm"
        >
          Delete
        </button>
      </div>
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
            ref="videoElement"
            controls
            playsinline
            class="max-h-full"
          >
            <source v-if="item.master_url" :src="item.master_url" />
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
          <audio
            v-else-if="item.type === 'audio' && (item.master_url || item.media_url)"
            controls
            class="w-full max-w-md"
          >
            <source v-if="item.master_url" :src="item.master_url" />
            <source v-else-if="item.media_url" :src="item.media_url" />
            Your browser does not support audio playback.
          </audio>
          <div
            v-else-if="item.type === 'audio' && !item.master_url && !item.media_url"
            class="absolute inset-0 flex flex-col items-center justify-center bg-black/80"
          >
            <span class="text-4xl mb-2">⏳</span>
            <span class="text-gray-300">Processing...</span>
            <span v-if="item.processing_status" class="text-gray-400 text-sm mt-1">
              Status: {{ item.processing_status }}
            </span>
          </div>
          <span v-else class="text-6xl">
            {{ item.type === 'video' ? '🎬' : item.type === 'audio' ? '🎵' : '📷' }}
          </span>
        </div>

        <div class="p-6">
          <div v-if="editing" class="mb-5 bg-gray-900/60 border border-gray-700 rounded-lg p-4 space-y-3">
            <h3 class="text-sm font-semibold text-gray-200">Edit Item</h3>
            <input
              v-model="editTitle"
              type="text"
              class="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white"
            />
            <textarea
              v-model="editDescription"
              rows="3"
              class="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-white resize-none"
            />
            <div class="flex gap-2">
              <button
                @click="saveEdit"
                :disabled="savingEdit || !editTitle.trim()"
                class="bg-emerald-700 hover:bg-emerald-600 disabled:bg-gray-600 text-white px-3 py-1.5 rounded-lg text-sm"
              >
                Save
              </button>
              <button
                @click="cancelEdit"
                class="bg-gray-700 hover:bg-gray-600 text-white px-3 py-1.5 rounded-lg text-sm"
              >
                Cancel
              </button>
            </div>
          </div>

          <div class="flex items-center gap-3 mb-2">
            <h1 class="text-2xl font-bold text-white flex-1">
              {{ item.title || 'Untitled' }}
            </h1>
            <button
              @click="toggleFavorite"
              :class="[
                'p-2 rounded-lg transition-colors',
                item.is_favorited ? 'text-red-500' : 'text-gray-500 hover:text-red-400'
              ]"
              title="Toggle favorite"
            >
              <svg class="w-6 h-6" :fill="item.is_favorited ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z"/>
              </svg>
            </button>
            <router-link
              :to="`/playlists?item=${encodeURIComponent(item.id)}`"
              class="p-2 rounded-lg transition-colors text-indigo-300 hover:text-indigo-200"
              title="Add to playlist"
            >
              <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h10" />
              </svg>
            </router-link>
            <button
              v-if="canManageItem"
              @click="toggleHighlight"
              :class="[
                'p-2 rounded-lg transition-colors',
                item.is_highlighted ? 'text-yellow-500' : 'text-gray-500 hover:text-yellow-400'
              ]"
              :title="item.is_highlighted ? 'Remove from featured' : 'Add to featured'"
            >
              <svg class="w-6 h-6" :fill="item.is_highlighted ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"/>
              </svg>
            </button>
            <span
              v-if="item.is_highlighted && !canManageItem"
              class="text-yellow-500"
              title="Featured"
            >
              <svg class="w-6 h-6" fill="currentColor" viewBox="0 0 24 24">
                <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
              </svg>
            </span>
            <span
              v-if="(item.type === 'video' || item.type === 'audio') && item.processing_status"
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
            <span
              v-if="item.hls_url"
              class="px-2 py-1 rounded text-xs font-medium bg-blue-600 text-white"
            >
              Streaming optimized
            </span>
            <span
              v-else-if="item.type === 'video' && item.processing_status === 'ready' && !item.hls_url"
              class="px-2 py-1 rounded text-xs font-medium bg-yellow-600 text-white"
            >
              Optimizing stream...
            </span>
          </div>
          <div class="flex items-center gap-4 mb-4">
            <PersonaBadge :display-name="getPersonaDisplayName(item)" :avatar-url="getPersonaAvatarUrl(item)" :slug="getPersonaSlug(item)" />
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
              <div class="flex items-center justify-between gap-2">
                <p class="text-gray-400 text-xs">{{ formatDuration(clip.duration_ms) }}</p>
                <button
                  v-if="canManageItem"
                  @click="removeClip(clip.clip_id)"
                  :disabled="deletingClipId === clip.clip_id"
                  class="text-red-300 hover:text-red-200 disabled:text-gray-500 text-xs"
                >
                  {{ deletingClipId === clip.clip_id ? 'Removing...' : 'Remove' }}
                </button>
              </div>
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
import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getItem, deleteItem, createClip, getItemClips, deleteClip, getPersonaDisplayName, getPersonaAvatarUrl, getPersonaSlug, setFavorite, setHighlight, getCurrentUser, updateItem, reprocessItem } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const route = useRoute()
const router = useRouter()

const item = ref(null)
const currentUser = ref(null)
const loading = ref(true)
const error = ref('')
const showDeleteConfirm = ref(false)
const clips = ref([])
const clipStart = ref(0)
const clipEnd = ref(10)
const clipCreating = ref(false)
const clipError = ref('')
const videoElement = ref(null)
const hlsPlayer = ref(null)
let hlsModulePromise = null
let statusPollTimer = null
let statusPollInFlight = false
const editing = ref(false)
const savingEdit = ref(false)
const reprocessing = ref(false)
const deletingClipId = ref('')
const editTitle = ref('')
const editDescription = ref('')

const isOwner = computed(() => {
  return currentUser.value && item.value && currentUser.value.id === item.value.user_id
})
const isAdmin = computed(() => currentUser.value?.role === 'admin')
const canManageItem = computed(() => Boolean(item.value?.can_manage) || isAdmin.value || isOwner.value)

async function toggleFavorite() {
  if (!item.value) return
  const newState = !item.value.is_favorited
  item.value.is_favorited = newState

  try {
    await setFavorite(item.value.id, newState)
  } catch (e) {
    item.value.is_favorited = !newState
    console.error('Failed to toggle favorite:', e)
  }
}

async function toggleHighlight() {
  if (!item.value || !canManageItem.value) return
  const newState = !item.value.is_highlighted
  item.value.is_highlighted = newState

  try {
    await setHighlight(item.value.id, newState)
  } catch (e) {
    item.value.is_highlighted = !newState
    console.error('Failed to toggle highlight:', e)
  }
}

async function getHlsModule() {
  if (!hlsModulePromise) {
    hlsModulePromise = import('hls.js/dist/hls.light.mjs')
  }
  return hlsModulePromise
}

async function initHLS() {
  if (!videoElement.value || !item.value?.hls_url) {
    return
  }

  const { default: Hls } = await getHlsModule()

  if (Hls.isSupported()) {
    if (hlsPlayer.value) {
      hlsPlayer.value.destroy()
    }
    const hls = new Hls({
      enableWorker: true,
      lowLatencyMode: false,
    })
    hls.loadSource(item.value.hls_url)
    hls.attachMedia(videoElement.value)
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      console.log('HLS manifest loaded')
    })
    hls.on(Hls.Events.ERROR, (event, data) => {
      console.error('HLS error:', data)
      if (data.fatal) {
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            console.log('Fatal network error, trying to recover...')
            hls.startLoad()
            break
          case Hls.ErrorTypes.MEDIA_ERROR:
            console.log('Fatal media error, trying to recover...')
            hls.recoverMediaError()
            break
          default:
            console.log('Fatal error, destroying hls player')
            hls.destroy()
            hlsPlayer.value = null
            break
        }
      }
    })
    hlsPlayer.value = hls
  } else if (videoElement.value.canPlayType('application/vnd.apple.mpegurl')) {
    videoElement.value.src = item.value.hls_url
  }
}

watch(
  () => item.value?.hls_url,
  async () => {
    await nextTick()
    await initHLS()
  },
  { flush: 'post' }
)

onMounted(loadItem)
onUnmounted(() => {
  stopStatusPolling()
  if (hlsPlayer.value) {
    hlsPlayer.value.destroy()
    hlsPlayer.value = null
  }
})

function shouldPollStatus() {
  if (!item.value) return false
  if (item.value.type !== 'video' && item.value.type !== 'audio') return false

  const status = (item.value.processing_status || '').toLowerCase()
  if (status === '' || status === 'queued' || status === 'processing') {
    return true
  }
  if (item.value.type === 'video' && status === 'ready' && !item.value.hls_url) {
    return true
  }
  return false
}

function stopStatusPolling() {
  if (statusPollTimer) {
    clearTimeout(statusPollTimer)
    statusPollTimer = null
  }
}

function scheduleStatusPolling() {
  stopStatusPolling()
  if (!shouldPollStatus()) return

  statusPollTimer = setTimeout(async () => {
    if (statusPollInFlight) {
      scheduleStatusPolling()
      return
    }

    statusPollInFlight = true
    try {
      await loadItem({ silent: true })
    } catch (_) {
      // ignore transient polling errors
    } finally {
      statusPollInFlight = false
      scheduleStatusPolling()
    }
  }, 3000)
}

async function loadItem(options = {}) {
  const silent = options.silent === true
  if (!silent) {
    loading.value = true
    error.value = ''
  }

  try {
    currentUser.value = await getCurrentUser()
    item.value = await getItem(route.params.id)
    await nextTick()
    await initHLS()
    scheduleStatusPolling()
    if (item.value.type === 'video') {
      await loadClips()
    }
  } catch (e) {
    if (!silent) {
      error.value = e.message
    }
  } finally {
    if (!silent) {
      loading.value = false
    }
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

function startEdit() {
  if (!item.value) return
  editTitle.value = item.value.title || ''
  editDescription.value = item.value.description || ''
  editing.value = true
}

function cancelEdit() {
  editing.value = false
  editTitle.value = ''
  editDescription.value = ''
}

async function saveEdit() {
  if (!item.value || !editTitle.value.trim()) return
  savingEdit.value = true
  try {
    await updateItem(item.value.id, {
      title: editTitle.value.trim(),
      description: editDescription.value.trim(),
    })
    item.value.title = editTitle.value.trim()
    item.value.description = editDescription.value.trim()
    cancelEdit()
  } catch (e) {
    error.value = e.message
  } finally {
    savingEdit.value = false
  }
}

async function triggerReprocess() {
  if (!item.value || reprocessing.value) return
  reprocessing.value = true
  try {
    await reprocessItem(item.value.id)
    await loadItem()
  } catch (e) {
    error.value = e.message
  } finally {
    reprocessing.value = false
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

async function removeClip(clipId) {
  if (!item.value || !clipId || deletingClipId.value) return
  deletingClipId.value = clipId
  clipError.value = ''
  try {
    await deleteClip(item.value.id, clipId)
    clips.value = clips.value.filter(c => c.clip_id !== clipId)
  } catch (e) {
    clipError.value = e.message
  } finally {
    deletingClipId.value = ''
  }
}

</script>
