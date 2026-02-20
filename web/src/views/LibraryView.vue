<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-6">
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
      <div class="space-y-4 mb-6">
        <div class="relative">
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search titles..."
            class="w-full bg-gray-800 text-white px-4 py-2 rounded-lg pl-10 focus:outline-none focus:ring-2 focus:ring-blue-500"
            @input="onSearchInput"
          />
          <svg class="absolute left-3 top-2.5 w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
          </svg>
        </div>

        <div class="flex flex-wrap gap-2">
          <select
            v-model="filterType"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            @change="updateURL"
          >
            <option value="">All Types</option>
            <option value="video">Videos</option>
            <option value="photo">Photos</option>
            <option value="audio">Audio</option>
          </select>

          <select
            v-model="filterPersona"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            @change="updateURL"
          >
            <option value="">All Personas</option>
            <option v-for="persona in personas" :key="persona.id" :value="persona.id">
              {{ persona.display_name }}
            </option>
          </select>

          <select
            v-model="sortOrder"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            @change="updateURL"
          >
            <option value="new">Newest First</option>
            <option value="old">Oldest First</option>
          </select>

          <button
            @click="toggleFavorites"
            :class="[
              'px-3 py-2 rounded-lg transition-colors flex items-center gap-2',
              showFavorites ? 'bg-red-600 text-white' : 'bg-gray-800 text-gray-400'
            ]"
          >
            <svg class="w-5 h-5" :fill="showFavorites ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z"/>
            </svg>
            Favorites
          </button>

          <button
            @click="toggleHighlighted"
            :class="[
              'px-3 py-2 rounded-lg transition-colors flex items-center gap-2',
              showHighlighted ? 'bg-yellow-600 text-white' : 'bg-gray-800 text-gray-400'
            ]"
          >
            <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
            </svg>
            Featured
          </button>
        </div>
      </div>

      <div v-if="loading" class="text-center text-gray-400 py-8">
        Loading...
      </div>

      <div v-else-if="error" class="text-center text-red-400 py-8">
        {{ error }}
      </div>

      <div v-else-if="items.length === 0" class="text-center text-gray-400 py-8">
        <p class="text-lg mb-4">{{ emptyStateMessage }}</p>
        <router-link v-if="!hasFilters" to="/upload" class="text-blue-400 hover:text-blue-300">
          Upload your first item →
        </router-link>
      </div>

      <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        <div
          v-for="item in items"
          :key="item.id"
          class="bg-[#0a2540] rounded-lg overflow-hidden hover:ring-2 hover:ring-blue-500 transition-all"
        >
          <router-link :to="`/item/${item.id}`" class="block">
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
              <span
                v-if="item.is_highlighted"
                class="absolute top-2 left-2 px-2 py-1 rounded text-xs font-medium bg-yellow-600/80 text-white"
              >
                ★
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
          <div class="px-3 pb-3 flex gap-2">
            <button
              @click.prevent="toggleFavorite(item)"
              :class="[
                'p-2 rounded-lg transition-colors',
                item.is_favorited ? 'text-red-500' : 'text-gray-500 hover:text-red-400'
              ]"
            >
              <svg class="w-5 h-5" :fill="item.is_favorited ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z"/>
              </svg>
            </button>
          </div>
        </div>
      </div>

      <div v-if="hasMore" class="text-center mt-6">
        <button 
          class="bg-blue-600 hover:bg-blue-500 px-6 py-2 rounded-lg"
          @click="loadMore"
          :disabled="loadingMore"
        >
          {{ loadingMore ? 'Loading...' : 'Load More' }}
        </button>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listItems, getPersonaDisplayName, getPersonaAvatarUrl, getPersonaSlug, setFavorite, getPersonas } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const route = useRoute()
const router = useRouter()

const items = ref([])
const personas = ref([])
const loading = ref(true)
const loadingMore = ref(false)
const error = ref('')
const hasMore = ref(false)
const cursor = ref('')

const searchQuery = ref('')
const filterType = ref('')
const filterPersona = ref('')
const sortOrder = ref('new')
const showFavorites = ref(false)
const showHighlighted = ref(false)

let searchDebounceTimer = null

const hasFilters = computed(() => {
  return searchQuery.value || filterType.value || filterPersona.value || showFavorites.value || showHighlighted.value
})

const emptyStateMessage = computed(() => {
  if (showFavorites.value) {
    return 'No favorites yet'
  }
  if (showHighlighted.value) {
    return 'No featured items'
  }
  if (searchQuery.value) {
    return 'No results found'
  }
  return 'No media yet'
})

function parseURLParams() {
  const q = route.query.q
  const type = route.query.type
  const persona_id = route.query.persona_id
  const favorites = route.query.favorites
  const highlighted = route.query.highlighted
  const sort = route.query.sort

  if (q !== undefined) searchQuery.value = q
  if (type !== undefined) filterType.value = type
  if (persona_id !== undefined) filterPersona.value = persona_id
  if (favorites === '1') showFavorites.value = true
  if (highlighted === '1') showHighlighted.value = true
  if (sort === 'old') sortOrder.value = 'old'
}

function updateURL() {
  const query = {}
  if (searchQuery.value) query.q = searchQuery.value
  if (filterType.value) query.type = filterType.value
  if (filterPersona.value) query.persona_id = filterPersona.value
  if (showFavorites.value) query.favorites = '1'
  if (showHighlighted.value) query.highlighted = '1'
  if (sortOrder.value === 'old') query.sort = 'old'
  
  router.replace({ query })
}

function onSearchInput() {
  clearTimeout(searchDebounceTimer)
  searchDebounceTimer = setTimeout(() => {
    cursor.value = ''
    loadItems()
    updateURL()
  }, 300)
}

function toggleFavorites() {
  showFavorites.value = !showFavorites.value
  cursor.value = ''
  loadItems()
  updateURL()
}

function toggleHighlighted() {
  showHighlighted.value = !showHighlighted.value
  cursor.value = ''
  loadItems()
  updateURL()
}

async function loadItems(append = false) {
  if (append) {
    loadingMore.value = true
  } else {
    loading.value = true
  }
  error.value = ''
  
  try {
    const options = {
      limit: 24,
      sort: sortOrder.value
    }
    if (searchQuery.value) options.q = searchQuery.value
    if (filterType.value) options.type = filterType.value
    if (filterPersona.value) options.persona_id = filterPersona.value
    if (showFavorites.value) options.favorites = '1'
    if (showHighlighted.value) options.highlighted = '1'
    if (append && cursor.value) options.cursor = cursor.value
    
    const result = await listItems(options)
    
    if (append) {
      items.value = [...items.value, ...result.items]
    } else {
      items.value = result.items
    }
    
    hasMore.value = result.has_more
    cursor.value = result.cursor || ''
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

async function loadMore() {
  await loadItems(true)
}

async function loadPersonas() {
  try {
    const result = await getPersonas()
    personas.value = result.personas || []
  } catch (e) {
    console.error('Failed to load personas:', e)
  }
}

async function toggleFavorite(item) {
  const newState = !item.is_favorited
  item.is_favorited = newState
  
  try {
    await setFavorite(item.id, newState)
  } catch (e) {
    item.is_favorited = !newState
    console.error('Failed to toggle favorite:', e)
  }
}

function formatDate(dateStr) {
  const date = new Date(dateStr)
  return date.toLocaleDateString()
}

watch([filterType, filterPersona, sortOrder], () => {
  cursor.value = ''
  loadItems()
})

onMounted(() => {
  parseURLParams()
  loadPersonas()
  loadItems()
})
</script>
