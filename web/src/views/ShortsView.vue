<template>
  <div class="min-h-screen bg-[#07182c] flex flex-col h-screen overflow-hidden">
    <router-link to="/" class="fixed top-4 left-4 z-50 bg-black/50 text-white px-4 py-2 rounded-lg">
      ← Back
    </router-link>
    
    <div v-if="loading && clips.length === 0" class="flex flex-col items-center justify-center h-full text-white">
      Loading shorts...
    </div>
    
    <div v-else-if="error" class="flex flex-col items-center justify-center h-full text-white">
      {{ error }}
    </div>
    
    <div v-else-if="clips.length === 0" class="flex flex-col items-center justify-center h-full text-white">
      <p class="mb-4">No shorts yet</p>
      <router-link to="/library" class="text-blue-400 hover:text-blue-300">
        Go to Library →
      </router-link>
    </div>
    
    <div v-else class="flex-1 relative overflow-hidden" @touchstart="handleTouchStart" @touchend="handleTouchEnd">
      <div 
        v-for="(clip, index) in clips" 
        :key="clip.clip_id"
        class="absolute inset-0 flex items-center justify-center transition-transform duration-300"
        :class="{ 'z-10': index === currentIndex }"
        :style="{ transform: `translateY(${(index - currentIndex) * 100}%)` }"
        @click="togglePlay"
      >
        <video
          v-if="index === currentIndex && clip.status === 'ready'"
          ref="videoPlayer"
          :src="clip.video_url"
          :poster="clip.thumb_url"
          class="max-h-screen max-w-full object-contain"
          loop
          playsinline
          muted
          @ended="playNext"
          @loadedmetadata="onVideoLoaded"
        ></video>
        
        <div v-else-if="clip.status !== 'ready'" class="flex flex-col items-center justify-center bg-black/80 text-white">
          <div class="w-10 h-10 border-4 border-white/30 border-t-white rounded-full animate-spin mb-4"></div>
          <p>Processing...</p>
        </div>
        
        <img 
          v-else 
          :src="clip.thumb_url" 
          :alt="clip.title"
          class="max-h-screen max-w-full object-contain"
        />
        
        <div v-if="index === currentIndex" class="absolute bottom-0 left-0 right-0 p-4 bg-gradient-to-t from-black/80 to-transparent">
          <div class="mb-2">
            <h3 class="text-white font-bold text-lg">{{ clip.title }}</h3>
            <div class="flex items-center gap-2 mt-1">
              <PersonaBadge :display-name="getPersonaDisplayName(clip)" :avatar-url="getPersonaAvatarUrl(clip)" variant="overlay" />
            </div>
          </div>
          
          <div class="flex gap-4 mt-4">
            <button 
              class="flex flex-col items-center text-white/80"
              :class="{ 'text-blue-500': clip.user_reaction === 1 }"
              @click.stop="react(clip, 1)"
            >
              <span class="text-2xl">👍</span>
              <span class="text-xs mt-1">{{ clip.up_count }}</span>
            </button>
            
            <button 
              class="flex flex-col items-center text-white/80"
              :class="{ 'text-blue-500': clip.user_reaction === -1 }"
              @click.stop="react(clip, -1)"
            >
              <span class="text-2xl">👎</span>
              <span class="text-xs mt-1">{{ clip.down_count }}</span>
            </button>
            
            <router-link 
              :to="`/item/${clip.item_id}`" 
              class="flex flex-col items-center text-white/80"
              @click.stop
            >
              <span class="text-2xl">ℹ️</span>
              <span class="text-xs mt-1">Info</span>
            </router-link>
          </div>
        </div>
        
        <div v-if="isPaused && index === currentIndex" class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-6xl text-white/80">
          ▶
        </div>
      </div>
    </div>
    
    <button v-if="hasMore" class="fixed bottom-4 left-1/2 -translate-x-1/2 bg-blue-600 text-white px-6 py-2 rounded-lg" @click="loadMore">
      Load More
    </button>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { listShorts, setReaction, getPersonaDisplayName, getPersonaAvatarUrl } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const clips = ref([])
const loading = ref(true)
const error = ref('')
const currentIndex = ref(0)
const hasMore = ref(false)
const cursor = ref('')
const isPaused = ref(false)
const videoPlayer = ref(null)
const touchStartY = ref(0)

async function loadShorts(append = false) {
  if (append) {
    loading.value = true
  }
  error.value = ''
  
  try {
    const result = await listShorts({ 
      limit: 10, 
      cursor: append ? cursor.value : undefined 
    })
    
    if (append) {
      clips.value = [...clips.value, ...result.clips]
    } else {
      clips.value = result.clips
    }
    
    hasMore.value = result.has_more
    cursor.value = result.cursor || ''
    
    if (clips.value.length > 0 && !append) {
      currentIndex.value = 0
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
  
  if (!append && clips.value.length > 0) {
    await nextTick()
    playCurrent()
  }
}

async function loadMore() {
  await loadShorts(true)
}

function playCurrent() {
  const video = videoPlayer.value?.[0]
  if (video && clips.value[currentIndex.value]?.status === 'ready') {
    video.currentTime = 0
    video.play().catch(() => {})
    isPaused.value = false
  }
}

function togglePlay() {
  const video = videoPlayer.value?.[0]
  if (!video) return
  
  if (video.paused) {
    video.play()
    isPaused.value = false
  } else {
    video.pause()
    isPaused.value = true
  }
}

function playNext() {
  if (currentIndex.value < clips.value.length - 1) {
    currentIndex.value++
    playCurrent()
  }
}

function playPrev() {
  if (currentIndex.value > 0) {
    currentIndex.value--
    playCurrent()
  }
}

function handleTouchStart(e) {
  touchStartY.value = e.touches[0].clientY
}

function handleTouchEnd(e) {
  const touchEndY = e.changedTouches[0].clientY
  const diff = touchStartY.value - touchEndY
  
  if (Math.abs(diff) > 50) {
    if (diff > 0) {
      playNext()
    } else {
      playPrev()
    }
  }
}

async function react(clip, value) {
  const currentReaction = clip.user_reaction === value ? 0 : value
  
  try {
    const result = await setReaction(clip.item_id, currentReaction)
    clip.up_count = result.up_count
    clip.down_count = result.down_count
    clip.user_reaction = result.user_reaction
  } catch (e) {
    console.error('Failed to react:', e)
  }
}

function onVideoLoaded() {
  const video = videoPlayer.value?.[0]
  if (video) {
    video.play().catch(() => {})
  }
}

onMounted(() => loadShorts)
</script>
