<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-8">
      <router-link to="/" class="text-gray-400 hover:text-white">
        ← Back
      </router-link>
      <h1 class="text-xl font-bold text-white">Personas</h1>
      <div class="w-16"></div>
    </header>

    <main class="max-w-lg mx-auto">
      <div class="bg-[#0a2540] rounded-lg p-6 space-y-6">
        <div>
          <h2 class="text-lg font-semibold text-white mb-4">Create New Persona</h2>
          <div class="flex gap-3">
            <input
              v-model="newPersonaName"
              type="text"
              placeholder="Persona name..."
              class="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-blue-500"
              @keyup.enter="doCreatePersona"
            />
            <button
              @click="doCreatePersona"
              :disabled="creating || !newPersonaName.trim()"
              class="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
            >
              {{ creating ? 'Creating...' : 'Create' }}
            </button>
          </div>
          <p v-if="error" class="text-red-400 text-sm mt-2">{{ error }}</p>
        </div>

        <div v-if="loading" class="text-center text-gray-400 py-4">
          Loading...
        </div>

        <div v-else-if="personas.length === 0" class="text-center text-gray-400 py-4">
          <p>No personas yet</p>
          <p class="text-sm mt-1">Create one above to get started</p>
        </div>

        <div v-else class="space-y-3">
          <h3 class="text-sm font-medium text-gray-400">Your Personas</h3>
          <div
            v-for="persona in personas"
            :key="persona.persona_id"
            class="flex items-center justify-between bg-gray-800 rounded-lg p-4"
          >
            <div class="flex items-center gap-3">
              <span class="w-10 h-10 rounded-full bg-purple-600 flex items-center justify-center text-white font-medium">
                {{ getInitials(persona.display_name) }}
              </span>
              <span class="text-white font-medium">{{ persona.display_name }}</span>
            </div>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getPersonas, createPersona } from '../services/api'

const personas = ref([])
const loading = ref(true)
const error = ref('')
const newPersonaName = ref('')
const creating = ref(false)

async function loadPersonas() {
  loading.value = true
  error.value = ''
  
  try {
    const result = await getPersonas()
    personas.value = result.personas || result || []
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function doCreatePersona() {
  if (!newPersonaName.value.trim()) return
  
  creating.value = true
  error.value = ''
  
  try {
    await createPersona(newPersonaName.value.trim())
    newPersonaName.value = ''
    await loadPersonas()
  } catch (e) {
    error.value = e.message
  } finally {
    creating.value = false
  }
}

function getInitials(name) {
  if (!name) return '?'
  const words = name.trim().split(/\s+/)
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase()
  }
  return name.slice(0, 2).toUpperCase()
}

onMounted(loadPersonas)
</script>
