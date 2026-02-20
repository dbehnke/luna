<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-8">
      <router-link to="/" class="text-gray-400 hover:text-white">
        ← Back
      </router-link>
      <h1 class="text-xl font-bold text-white">Profile</h1>
      <div class="w-16"></div>
    </header>

    <main class="max-w-2xl mx-auto space-y-6">
      <section class="bg-[#0a2540] rounded-lg p-6">
        <p class="text-sm text-gray-400 mb-1">Signed in as</p>
        <p class="text-lg font-semibold text-white">
          {{ currentUser?.username || "Unknown user" }}
        </p>
        <p class="text-sm text-gray-400 mt-1">Role: {{ currentUser?.role || "user" }}</p>
      </section>

      <section class="bg-[#0a2540] rounded-lg p-6 space-y-6">
        <div>
          <h2 class="text-lg font-semibold text-white mb-4">Create Persona</h2>
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
              {{ creating ? "Creating..." : "Create" }}
            </button>
          </div>
        </div>

        <div v-if="loading" class="text-center text-gray-400 py-4">
          Loading personas...
        </div>

        <div v-else-if="personas.length === 0" class="text-center text-gray-400 py-4">
          <p>No personas yet</p>
          <p class="text-sm mt-1">Create one above to get started</p>
        </div>

        <div v-else class="space-y-3">
          <h3 class="text-sm font-medium text-gray-400">Your personas</h3>
          <div
            v-for="persona in personas"
            :key="persona.id"
            class="flex items-center justify-between bg-gray-800 rounded-lg p-4"
          >
            <div class="flex items-center gap-3">
              <div class="relative">
                <img
                  v-if="persona.avatar_url"
                  :src="persona.avatar_url"
                  class="w-10 h-10 rounded-full object-cover"
                  alt="Avatar"
                />
                <span
                  v-else
                  class="w-10 h-10 rounded-full bg-blue-600 flex items-center justify-center text-white font-medium"
                >
                  {{ getInitials(persona.display_name) }}
                </span>
                <label
                  class="absolute -bottom-1 -right-1 w-6 h-6 bg-blue-600 rounded-full flex items-center justify-center cursor-pointer hover:bg-blue-700"
                >
                  <input
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    class="hidden"
                    :disabled="uploading"
                    @change="(e) => handleAvatarUpload(persona.id, e)"
                  />
                  <svg class="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 13a3 3 0 11-6 0 3 3 0 016 0z" />
                  </svg>
                </label>
              </div>
              <span class="text-white font-medium">{{ persona.display_name }}</span>
            </div>
            <router-link
              :to="`/@${persona.slug}`"
              class="text-xs text-blue-300 hover:text-blue-200"
            >
              View
            </router-link>
          </div>
        </div>

        <p v-if="error" class="text-red-400 text-sm">{{ error }}</p>
      </section>
    </main>
  </div>
</template>

<script setup>
import { onMounted, ref } from "vue";
import {
  createPersona,
  getCurrentUser,
  getPersonas,
  uploadPersonaAvatar,
} from "../services/api";

const currentUser = ref(null);
const personas = ref([]);
const loading = ref(true);
const creating = ref(false);
const uploading = ref(false);
const error = ref("");
const newPersonaName = ref("");

async function loadCurrentUser() {
  currentUser.value = await getCurrentUser();
}

async function loadPersonas() {
  loading.value = true;
  error.value = "";

  try {
    const result = await getPersonas();
    personas.value = result.personas || [];
  } catch (e) {
    error.value = e.message || "Failed to load personas";
  } finally {
    loading.value = false;
  }
}

async function doCreatePersona() {
  if (!newPersonaName.value.trim()) return;

  creating.value = true;
  error.value = "";

  try {
    await createPersona(newPersonaName.value.trim());
    newPersonaName.value = "";
    await loadPersonas();
  } catch (e) {
    error.value = e.message || "Failed to create persona";
  } finally {
    creating.value = false;
  }
}

async function handleAvatarUpload(personaId, event) {
  const file = event.target.files?.[0];
  if (!file) return;

  uploading.value = true;
  error.value = "";
  try {
    await uploadPersonaAvatar(personaId, file);
    await loadPersonas();
  } catch (e) {
    error.value = e.message || "Failed to upload avatar";
  } finally {
    uploading.value = false;
    event.target.value = "";
  }
}

function getInitials(name) {
  if (!name) return "?";
  const words = name.trim().split(/\s+/);
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

onMounted(async () => {
  await Promise.all([loadCurrentUser(), loadPersonas()]);
});
</script>
