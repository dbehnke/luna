<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-8">
      <router-link to="/" class="text-gray-400 hover:text-white">
        ← Back
      </router-link>
      <h1 class="text-xl font-bold text-white">Upload Media</h1>
      <div class="w-16"></div>
    </header>

    <main class="max-w-lg mx-auto">
      <div v-if="!itemId" class="space-y-6">
        <div class="bg-[#0a2540] rounded-lg p-6 space-y-4">
          <h2 class="text-lg font-semibold text-white">Create New Item</h2>
          
          <div>
            <label class="block text-sm text-gray-400 mb-2">Type</label>
            <div class="flex gap-4">
              <button
                @click="mediaType = 'video'"
                :class="[
                  'flex-1 py-3 rounded-lg font-medium transition-colors',
                  mediaType === 'video' 
                    ? 'bg-blue-600 text-white' 
                    : 'bg-gray-700 text-gray-300'
                ]"
              >
                Video
              </button>
              <button
                @click="mediaType = 'photo'"
                :class="[
                  'flex-1 py-3 rounded-lg font-medium transition-colors',
                  mediaType === 'photo' 
                    ? 'bg-blue-600 text-white' 
                    : 'bg-gray-700 text-gray-300'
                ]"
              >
                Photo
              </button>
            </div>
          </div>

          <div>
            <label class="block text-sm text-gray-400 mb-2">Title (optional)</label>
            <input
              v-model="title"
              type="text"
              placeholder="Enter title..."
              class="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-blue-500"
            />
          </div>

          <div v-if="!loadingPersonas && personas.length > 0">
            <label class="block text-sm text-gray-400 mb-2">Upload as</label>
            <div class="flex gap-3 items-center">
              <select
                v-model="selectedPersonaId"
                @change="updateSelectedPersona"
                class="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-blue-500"
              >
                <option :value="null">Self</option>
                <option
                  v-for="persona in personas"
                  :key="persona.persona_id"
                  :value="persona.persona_id"
                >
                  {{ persona.display_name }}
                </option>
              </select>
              <PersonaBadge
                v-if="selectedPersonaId !== null"
                :display-name="selectedPersona?.display_name"
                :avatar-url="selectedPersona?.avatar_url"
              />
            </div>
          </div>

          <div>
            <label class="block text-sm text-gray-400 mb-2">Description (optional)</label>
            <textarea
              v-model="description"
              placeholder="Enter description..."
              rows="3"
              class="w-full bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-blue-500 resize-none"
            ></textarea>
          </div>

          <button
            @click="doCreateItem"
            :disabled="creating"
            class="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white font-medium py-3 rounded-lg transition-colors"
          >
            {{ creating ? 'Creating...' : 'Continue to Upload' }}
          </button>

          <p v-if="error" class="text-red-400 text-sm">{{ error }}</p>
        </div>
      </div>

      <div v-else class="space-y-6">
        <div class="bg-[#0a2540] rounded-lg p-6 space-y-4">
          <h2 class="text-lg font-semibold text-white">Upload File</h2>
          <p class="text-gray-400">Selected: {{ title || 'Untitled' }}</p>
          
          <div
            @dragover.prevent="dragOver = true"
            @dragleave.prevent="dragOver = false"
            @drop.prevent="handleDrop"
            :class="[
              'border-2 border-dashed rounded-lg p-8 text-center transition-colors',
              dragOver ? 'border-blue-500 bg-blue-900/20' : 'border-gray-600'
            ]"
          >
            <input
              ref="fileInput"
              type="file"
              :accept="mediaType === 'video' ? 'video/*' : 'image/*'"
              @change="handleFileSelect"
              class="hidden"
            />
            
            <div v-if="!selectedFile" class="space-y-2">
              <p class="text-gray-400">
                {{ dragOver ? 'Drop file here' : 'Drag & drop or click to select' }}
              </p>
              <p class="text-sm text-gray-500">
                {{ mediaType === 'video' ? 'Video files supported' : 'Image files supported' }}
              </p>
              <button
                @click="$refs.fileInput.click()"
                class="mt-4 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors"
              >
                Select File
              </button>
            </div>
            
            <div v-else class="space-y-2">
              <p class="text-white font-medium">{{ selectedFile.name }}</p>
              <p class="text-sm text-gray-400">{{ formatSize(selectedFile.size) }}</p>
              <button
                @click="selectedFile = null"
                class="text-red-400 hover:text-red-300 text-sm"
              >
                Remove
              </button>
            </div>
          </div>

          <button
            @click="doUploadFile"
            :disabled="uploading || !selectedFile"
            class="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white font-medium py-3 rounded-lg transition-colors"
          >
            {{ uploading ? 'Uploading...' : 'Upload' }}
          </button>

          <p v-if="uploadError" class="text-red-400 text-sm">{{ uploadError }}</p>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { createItem, uploadFile, getPersonas, getPersonaAvatarUrl } from '../services/api'
import PersonaBadge from '../components/PersonaBadge.vue'

const router = useRouter()

const mediaType = ref('video')
const title = ref('')
const description = ref('')
const creating = ref(false)
const error = ref('')

const personas = ref([])
const selectedPersonaId = ref(null)
const loadingPersonas = ref(true)

onMounted(async () => {
  try {
    const result = await getPersonas()
    personas.value = result.personas || result || []
  } catch (e) {
    console.error('Failed to load personas:', e)
  } finally {
    loadingPersonas.value = false
  }
})

const selectedPersona = ref(null)
function updateSelectedPersona() {
  if (selectedPersonaId.value === null) {
    selectedPersona.value = null
  } else {
    selectedPersona.value = personas.value.find(p => p.persona_id === selectedPersonaId.value)
  }
}

const itemId = ref(null)
const selectedFile = ref(null)
const fileInput = ref(null)
const dragOver = ref(false)
const uploading = ref(false)
const uploadError = ref('')

async function doCreateItem() {
  creating.value = true
  error.value = ''
  
  try {
    const result = await createItem(mediaType.value, title.value, description.value, selectedPersonaId.value)
    itemId.value = result.item_id
  } catch (e) {
    error.value = e.message
  } finally {
    creating.value = false
  }
}

function handleFileSelect(e) {
  const file = e.target.files[0]
  if (file) {
    selectedFile.value = file
  }
}

function handleDrop(e) {
  dragOver.value = false
  const file = e.dataTransfer.files[0]
  if (file) {
    const isVideo = file.type.startsWith('video/')
    const isImage = file.type.startsWith('image/')
    if ((mediaType.value === 'video' && isVideo) || (mediaType.value === 'photo' && isImage)) {
      selectedFile.value = file
    }
  }
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

async function doUploadFile() {
  uploading.value = true
  uploadError.value = ''
  
  try {
    const result = await uploadFile(itemId.value, selectedFile.value)
    router.push(`/item/${itemId.value}`)
  } catch (e) {
    uploadError.value = e.message
  } finally {
    uploading.value = false
  }
}
</script>
