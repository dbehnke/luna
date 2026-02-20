<template>
  <div class="min-h-screen bg-[#0a1628] text-white">
    <router-link to="/" class="fixed top-4 left-4 z-50 bg-black/50 text-white px-4 py-2 rounded-lg">
      ← Back
    </router-link>
    
    <div v-if="loading && !profile" class="flex flex-col items-center justify-center h-screen text-white">
      Loading profile...
    </div>
    
    <div v-else-if="error" class="flex flex-col items-center justify-center h-screen text-white">
      <p class="text-red-400 mb-4">{{ error }}</p>
      <router-link to="/" class="text-blue-400 hover:text-blue-300">
        Go Home
      </router-link>
    </div>
    
    <div v-else-if="profile">
      <div class="p-6 pt-16">
        <div class="flex items-center gap-4 mb-6">
          <img
            v-if="profile.avatar_url"
            :src="profile.avatar_url"
            :alt="profile.display_name"
            class="w-20 h-20 rounded-full object-cover border-2 border-purple-500"
          />
          <div
            v-else
            class="w-20 h-20 rounded-full flex items-center justify-center text-2xl font-bold bg-purple-600 border-2 border-purple-500"
          >
            {{ initials }}
          </div>
          
          <div>
            <h1 class="text-2xl font-bold">{{ profile.display_name }}</h1>
            <div class="flex gap-4 mt-2 text-gray-400">
              <span>{{ profile.counts.videos }} videos</span>
              <span>{{ profile.counts.shorts }} shorts</span>
            </div>
          </div>
        </div>
        
        <div class="flex border-b border-gray-700 mb-6">
          <button
            :class="[
              'flex-1 py-3 text-center font-medium transition-colors',
              activeTab === 'videos' 
                ? 'text-purple-400 border-b-2 border-purple-400' 
                : 'text-gray-400 hover:text-white'
            ]"
            @click="activeTab = 'videos'"
          >
            Videos
          </button>
          <button
            :class="[
              'flex-1 py-3 text-center font-medium transition-colors',
              activeTab === 'shorts' 
                ? 'text-purple-400 border-b-2 border-purple-400' 
                : 'text-gray-400 hover:text-white'
            ]"
            @click="activeTab = 'shorts'"
          >
            Shorts
          </button>
        </div>
        
        <div v-if="activeTab === 'videos'">
          <div class="flex flex-wrap gap-2 mb-4">
            <div class="relative flex-1 min-w-[200px]">
              <input
                v-model="searchQuery"
                type="text"
                placeholder="Search videos..."
                class="w-full bg-[#162a4a] text-white px-3 py-2 rounded-lg pl-9 focus:outline-none focus:ring-2 focus:ring-purple-500 text-sm"
                @input="onSearchInput"
              />
              <svg class="absolute left-3 top-2.5 w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
              </svg>
            </div>
            
            <select
              v-model="sortOrder"
              class="bg-[#162a4a] text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 text-sm"
              @change="loadVideos()"
            >
              <option value="new">Newest First</option>
              <option value="old">Oldest First</option>
            </select>
          </div>
          
          <div v-if="videosLoading" class="text-center text-gray-400 py-8">
            Loading videos...
          </div>
          
          <div v-else-if="videos.length === 0" class="text-center text-gray-400 py-8">
            No videos yet
          </div>
          
          <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
            <router-link
              v-for="video in videos"
              :key="video.id"
              :to="`/item/${video.id}`"
              class="block bg-[#162a4a] rounded-lg overflow-hidden hover:bg-[#1e3a5f] transition-colors"
            >
              <div class="aspect-video bg-black relative">
                <img
                  v-if="video.thumb_url"
                  :src="video.thumb_url"
                  :alt="video.title"
                  class="w-full h-full object-cover"
                />
                <div v-else class="w-full h-full flex items-center justify-center text-gray-500">
                  <svg class="w-12 h-12" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M8 5v14l11-7z"/>
                  </svg>
                </div>
              </div>
              <div class="p-3">
                <h3 class="font-medium text-sm truncate">{{ video.title }}</h3>
                <p class="text-xs text-gray-400 mt-1">{{ formatDate(video.created_at) }}</p>
              </div>
            </router-link>
          </div>
          
          <div v-if="videosHasMore" class="text-center mt-6">
            <button 
              class="bg-purple-600 hover:bg-purple-500 px-6 py-2 rounded-lg"
              @click="loadMoreVideos"
            >
              Load More
            </button>
          </div>
        </div>
        
        <div v-if="activeTab === 'shorts'">
          <div v-if="shortsLoading" class="text-center text-gray-400 py-8">
            Loading shorts...
          </div>
          
          <div v-else-if="shorts.length === 0" class="text-center text-gray-400 py-8">
            No shorts yet
          </div>
          
          <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
            <div
              v-for="short in shorts"
              :key="short.clip_id"
              class="block bg-[#162a4a] rounded-lg overflow-hidden hover:bg-[#1e3a5f] transition-colors cursor-pointer"
              @click="goToShorts(short.clip_id)"
            >
              <div class="aspect-[9/16] bg-black relative">
                <img
                  v-if="short.thumb_url"
                  :src="short.thumb_url"
                  :alt="short.title"
                  class="w-full h-full object-cover"
                />
                <div v-else class="w-full h-full flex items-center justify-center text-gray-500">
                  <svg class="w-12 h-12" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M8 5v14l11-7z"/>
                  </svg>
                </div>
                <div class="absolute bottom-2 right-2 bg-black/70 px-2 py-1 rounded text-xs">
                  {{ formatDuration(short) }}
                </div>
              </div>
              <div class="p-3">
                <h3 class="font-medium text-sm truncate">{{ short.title }}</h3>
                <p class="text-xs text-gray-400 mt-1">{{ formatDate(short.created_at) }}</p>
              </div>
            </div>
          </div>
          
          <div v-if="shortsHasMore" class="text-center mt-6">
            <button 
              class="bg-purple-600 hover:bg-purple-500 px-6 py-2 rounded-lg"
              @click="loadMoreShorts"
            >
              Load More
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getProfile, getProfileItems, getProfileShorts } from '../services/api'

const route = useRoute()
const slug = computed(() => route.params.slug)

const profile = ref(null)
const loading = ref(true)
const error = ref('')
const activeTab = ref('videos')

const searchQuery = ref('')
const sortOrder = ref('new')

let searchDebounceTimer = null

const videos = ref([])
const videosLoading = ref(false)
const videosHasMore = ref(false)
const videosCursor = ref('')

const shorts = ref([])
const shortsLoading = ref(false)
const shortsHasMore = ref(false)
const shortsCursor = ref('')

const initials = computed(() => {
  if (!profile.value?.display_name) return '?'
  const words = profile.value.display_name.trim().split(/\s+/)
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase()
  }
  return profile.value.display_name.slice(0, 2).toUpperCase()
})

async function loadProfile() {
  loading.value = true
  error.value = ''
  
  try {
    profile.value = await getProfile(slug.value)
  } catch (e) {
    error.value = e.message || 'Failed to load profile'
  } finally {
    loading.value = false
  }
}

async function loadVideos(append = false) {
  if (append) {
    videosLoading.value = true
  }
  
  try {
    const options = {
      type: 'video',
      limit: 24,
      sort: sortOrder.value
    }
    if (searchQuery.value) options.q = searchQuery.value
    if (append && videosCursor.value) options.cursor = videosCursor.value
    
    const result = await getProfileItems(slug.value, options)
    
    if (append) {
      videos.value = [...videos.value, ...result.items]
    } else {
      videos.value = result.items
    }
    
    videosHasMore.value = result.has_more
    videosCursor.value = result.next_cursor || ''
  } catch (e) {
    console.error('Failed to load videos:', e)
  } finally {
    videosLoading.value = false
  }
}

function onSearchInput() {
  clearTimeout(searchDebounceTimer)
  searchDebounceTimer = setTimeout(() => {
    videosCursor.value = ''
    loadVideos()
  }, 300)
}

async function loadShorts(append = false) {
  if (append) {
    shortsLoading.value = true
  }
  
  try {
    const result = await getProfileShorts(slug.value, {
      limit: 12,
      cursor: append ? shortsCursor.value : undefined
    })
    
    if (append) {
      shorts.value = [...shorts.value, ...result.shorts]
    } else {
      shorts.value = result.shorts
    }
    
    shortsHasMore.value = result.has_more
    shortsCursor.value = result.next_cursor || ''
  } catch (e) {
    console.error('Failed to load shorts:', e)
  } finally {
    shortsLoading.value = false
  }
}

async function loadMoreVideos() {
  await loadVideos(true)
}

async function loadMoreShorts() {
  await loadShorts(true)
}

function goToShorts(clipId) {
  window.location.href = `/shorts?clip=${clipId}`
}

function formatDate(dateStr) {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatDuration(short) {
  if (!short.duration_ms) return ''
  const seconds = Math.floor(short.duration_ms / 1000)
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  return `${mins}:${secs.toString().padStart(2, '0')}`
}

watch(activeTab, async (tab) => {
  if (tab === 'videos' && videos.value.length === 0) {
    await loadVideos()
  } else if (tab === 'shorts' && shorts.value.length === 0) {
    await loadShorts()
  }
})

watch(slug, async () => {
  await loadProfile()
  videos.value = []
  shorts.value = []
  searchQuery.value = ''
  sortOrder.value = 'new'
  activeTab.value = 'videos'
})

onMounted(async () => {
  await loadProfile()
  if (profile.value) {
    await loadVideos()
  }
})
</script>
