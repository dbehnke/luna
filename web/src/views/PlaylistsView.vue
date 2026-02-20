<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-6">
      <router-link to="/" class="text-gray-400 hover:text-white"> ← Back </router-link>
      <h1 class="text-xl font-bold text-white">Playlists</h1>
      <button
        class="bg-gray-700 hover:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
        @click="load"
      >
        Refresh
      </button>
    </header>

    <main class="space-y-6 max-w-4xl mx-auto">
      <section class="bg-[#0a2540] rounded-lg p-4">
        <h2 class="text-white font-semibold mb-3">Create Playlist</h2>
        <form class="flex gap-3" @submit.prevent="submitCreate">
          <input
            v-model="newName"
            type="text"
            placeholder="Playlist name..."
            class="flex-1 bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            required
          />
          <button
            class="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
            :disabled="busy || !newName.trim()"
          >
            Create
          </button>
        </form>
      </section>

      <section v-if="pendingItemId" class="bg-[#0a2540] rounded-lg p-4 space-y-3">
        <h2 class="text-white font-semibold">Add Item To Playlist</h2>
        <p class="text-sm text-gray-300">Item: <span class="font-mono">{{ pendingItemId }}</span></p>
        <div class="flex gap-3">
          <select
            v-model="selectedPlaylistId"
            class="flex-1 bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="">Select playlist</option>
            <option v-for="p in playlists" :key="p.id" :value="String(p.id)">
              {{ p.name }}
            </option>
          </select>
          <button
            class="bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
            :disabled="busy || !selectedPlaylistId"
            @click="submitAddPendingItem"
          >
            Add
          </button>
        </div>
      </section>

      <section v-if="error" class="text-red-400 text-sm">
        {{ error }}
      </section>
      <section v-if="message" class="text-green-400 text-sm">
        {{ message }}
      </section>

      <section
        v-for="playlist in playlists"
        :key="playlist.id"
        class="bg-[#0a2540] rounded-lg p-4 space-y-3"
      >
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-white font-semibold">{{ playlist.name }}</h3>
            <p class="text-xs text-gray-400">{{ playlist.item_count }} items</p>
          </div>
          <button
            class="text-red-400 hover:text-red-300 text-sm"
            :disabled="busy"
            @click="deleteOne(playlist.id)"
          >
            Delete
          </button>
        </div>

        <div v-if="!playlist.items || playlist.items.length === 0" class="text-sm text-gray-400">
          No items yet
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="item in playlist.items"
            :key="`${playlist.id}-${item.item_id}`"
            class="bg-gray-800 rounded-lg p-3 flex items-center justify-between"
          >
            <div class="min-w-0">
              <p class="text-white truncate">{{ item.title || item.item_id }}</p>
              <p class="text-xs text-gray-400">{{ item.type }} · position {{ item.position + 1 }}</p>
            </div>
            <div class="flex items-center gap-2">
              <router-link
                :to="`/item/${item.item_id}`"
                class="text-blue-300 hover:text-blue-200 text-sm"
              >
                Open
              </router-link>
              <button
                class="text-red-400 hover:text-red-300 text-sm"
                :disabled="busy"
                @click="removeItem(playlist.id, item.item_id)"
              >
                Remove
              </button>
            </div>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup>
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  addPlaylistItem,
  createPlaylist,
  deletePlaylist,
  listPlaylists,
  removePlaylistItem,
} from "../services/api";

const route = useRoute();
const router = useRouter();

const playlists = ref([]);
const newName = ref("");
const selectedPlaylistId = ref("");
const pendingItemId = ref("");
const error = ref("");
const message = ref("");
const busy = ref(false);

async function load() {
  error.value = "";
  const result = await listPlaylists(true);
  playlists.value = result.playlists || [];
}

async function submitCreate() {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await createPlaylist(newName.value.trim());
    newName.value = "";
    message.value = "Playlist created";
    await load();
  } catch (e) {
    error.value = e.message || "Failed to create playlist";
  } finally {
    busy.value = false;
  }
}

async function deleteOne(id) {
  if (!window.confirm("Delete this playlist?")) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await deletePlaylist(id);
    message.value = "Playlist deleted";
    await load();
  } catch (e) {
    error.value = e.message || "Failed to delete playlist";
  } finally {
    busy.value = false;
  }
}

async function removeItem(playlistId, itemId) {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await removePlaylistItem(playlistId, itemId);
    message.value = "Item removed";
    await load();
  } catch (e) {
    error.value = e.message || "Failed to remove item";
  } finally {
    busy.value = false;
  }
}

async function submitAddPendingItem() {
  if (!pendingItemId.value || !selectedPlaylistId.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await addPlaylistItem(Number(selectedPlaylistId.value), pendingItemId.value);
    message.value = "Item added to playlist";
    pendingItemId.value = "";
    selectedPlaylistId.value = "";
    await router.replace({ path: "/playlists" });
    await load();
  } catch (e) {
    error.value = e.message || "Failed to add item";
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  const qItem = route.query.item;
  if (typeof qItem === "string" && qItem.trim()) {
    pendingItemId.value = qItem.trim();
  }
  try {
    await load();
  } catch (e) {
    error.value = e.message || "Failed to load playlists";
  }
});
</script>
