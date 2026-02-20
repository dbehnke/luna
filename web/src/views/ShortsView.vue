<template>
  <div class="min-h-screen bg-[#07182c] flex flex-col h-screen overflow-hidden relative">
    <router-link to="/" class="absolute top-4 left-4 z-50 bg-black/50 text-white px-4 py-2 rounded-lg">
      ← Back
    </router-link>
    <div class="absolute top-4 right-4 z-50 flex items-center gap-2 bg-black/50 p-1 rounded-lg">
      <button
        @click="viewMode = 'feed'"
        :class="[
          'px-3 py-1 rounded text-sm',
          viewMode === 'feed' ? 'bg-blue-600 text-white' : 'text-gray-200 hover:bg-white/10'
        ]"
      >
        Feed
      </button>
      <button
        @click="viewMode = 'grid'"
        :class="[
          'px-3 py-1 rounded text-sm',
          viewMode === 'grid' ? 'bg-blue-600 text-white' : 'text-gray-200 hover:bg-white/10'
        ]"
      >
        Grid
      </button>
    </div>

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

    <div
      v-else-if="viewMode === 'feed'"
      class="flex-1 relative overflow-hidden"
      @touchstart="handleTouchStart"
      @touchend="handleTouchEnd"
    >
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
              <PersonaBadge :display-name="getPersonaDisplayName(clip)" :avatar-url="getPersonaAvatarUrl(clip)" :slug="getPersonaSlug(clip)" variant="overlay" />
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

            <router-link
              :to="`/playlists?item=${encodeURIComponent(clip.item_id)}`"
              class="flex flex-col items-center text-white/80"
              @click.stop
            >
              <span class="text-2xl">📁</span>
              <span class="text-xs mt-1">Playlist</span>
            </router-link>
          </div>
        </div>

        <div v-if="isPaused && index === currentIndex" class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-6xl text-white/80">
          ▶
        </div>
      </div>

      <button
        class="hidden md:flex absolute left-4 top-1/2 -translate-y-1/2 z-20 w-12 h-12 items-center justify-center rounded-full bg-black/50 text-white text-2xl hover:bg-black/70"
        @click.stop="playPrev"
        title="Previous short"
      >
        ‹
      </button>
      <button
        class="hidden md:flex absolute right-4 top-1/2 -translate-y-1/2 z-20 w-12 h-12 items-center justify-center rounded-full bg-black/50 text-white text-2xl hover:bg-black/70"
        @click.stop="playNext"
        title="Next short"
      >
        ›
      </button>
    </div>

    <div v-else class="flex-1 overflow-y-auto p-16 pt-20">
      <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-2">
        <div
          v-for="clip in clips"
          :key="clip.clip_id"
          class="relative bg-black rounded-md overflow-hidden aspect-[9/16] border border-black/40"
        >
          <video
            v-if="clip.status === 'ready'"
            :src="clip.video_url"
            :poster="clip.thumb_url"
            class="w-full h-full object-cover"
            playsinline
            preload="metadata"
            @mouseenter="playGridPreview($event)"
            @mouseleave="stopGridPreview($event)"
            @click="openClip(clip.clip_id)"
          ></video>
          <div
            v-else
            class="w-full h-full flex flex-col items-center justify-center text-gray-300 text-sm"
          >
            <div class="w-8 h-8 border-2 border-gray-400 border-t-transparent rounded-full animate-spin mb-2"></div>
            Processing
          </div>
          <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/80 to-transparent pointer-events-none">
            <p class="text-white text-xs truncate">{{ clip.title }}</p>
          </div>
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
import { useRoute } from 'vue-router'
import { listShorts, setReaction, getPersonaDisplayName, getPersonaAvatarUrl, getPersonaSlug } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const route = useRoute()

const clips = ref([])
const loading = ref(true)
const error = ref('')
const currentIndex = ref(0)
const hasMore = ref(false)
const cursor = ref('')
const isPaused = ref(false)
const videoPlayer = ref(null)
const touchStartY = ref(0)
const targetClipId = ref(null)
const viewMode = ref('feed')

function findClipIndex(clips, clipId) {
  return clips.findIndex(c => c.clip_id === clipId)
}

function jumpToClip(clipId) {
  const idx = findClipIndex(clips.value, clipId)
  if (idx >= 0) {
    currentIndex.value = idx
    nextTick(() => playCurrent())
  } else if (hasMore.value) {
    loadMoreWithTarget(clipId)
  }
}

async function loadMoreWithTarget(targetClip) {
  loading.value = true
  try {
    const result = await listShorts({
      limit: 10,
      cursor: cursor.value
    })
    clips.value = [...clips.value, ...result.clips]
    hasMore.value = result.has_more
    cursor.value = result.cursor || ''

    const newIdx = findClipIndex(clips.value, targetClip)
    if (newIdx >= 0) {
      currentIndex.value = newIdx
      await nextTick()
      playCurrent()
    } else if (hasMore.value) {
      await loadMoreWithTarget(targetClip)
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

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

    if (targetClipId.value) {
      const idx = findClipIndex(clips.value, targetClipId.value)
      if (idx >= 0) {
        currentIndex.value = idx
        targetClipId.value = null
      } else if (hasMore.value && !append) {
        await loadMoreWithTarget(targetClipId.value)
        return
      }
    } else if (clips.value.length > 0 && !append) {
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
    video.muted = false
    video.currentTime = 0
    video.play().catch(() => {
      video.muted = true
      void video.play().catch(() => {})
    })
    isPaused.value = false
  }
}

function togglePlay() {
  const video = videoPlayer.value?.[0]
  if (!video) return

  if (video.paused) {
    video.muted = false
    video.play().catch(() => {
      video.muted = true
      void video.play().catch(() => {})
    })
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
    video.muted = false
    video.play().catch(() => {
      video.muted = true
      void video.play().catch(() => {})
    })
  }
}

function playGridPreview(event) {
  const video = event?.target
  if (!(video instanceof HTMLVideoElement)) return
  video.muted = false
  video.currentTime = 0
  const playPromise = video.play()
  if (playPromise && typeof playPromise.catch === 'function') {
    playPromise.catch(() => {
      video.muted = true
      void video.play().catch(() => {})
    })
  }
}

function stopGridPreview(event) {
  const video = event?.target
  if (!(video instanceof HTMLVideoElement)) return
  video.pause()
  video.currentTime = 0
}

function openClip(clipId) {
  if (!clipId) return
  targetClipId.value = clipId
  viewMode.value = 'feed'
  jumpToClip(clipId)
}

onMounted(() => {
  const clipParam = route.query.clip
  if (clipParam) {
    targetClipId.value = clipParam
  }
  loadShorts()
})
</script>
