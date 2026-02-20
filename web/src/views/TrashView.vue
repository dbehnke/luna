<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-6">
      <router-link to="/" class="text-gray-400 hover:text-white"> ← Back </router-link>
      <h1 class="text-xl font-bold text-white">Trash</h1>
      <button
        class="bg-gray-700 hover:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
        @click="load"
      >
        Refresh
      </button>
    </header>

    <main class="max-w-4xl mx-auto space-y-4">
      <section class="bg-[#0a2540] rounded-lg p-4 flex items-center justify-between">
        <p class="text-sm text-gray-300">
          Soft-deleted items can be restored. Admins can permanently purge items.
        </p>
        <select
          v-if="isAdmin"
          v-model="scope"
          class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
          @change="load"
        >
          <option value="mine">My trash</option>
          <option value="all">All users</option>
        </select>
      </section>

      <section v-if="error" class="text-red-400 text-sm">
        {{ error }}
      </section>
      <section v-if="message" class="text-green-400 text-sm">
        {{ message }}
      </section>

      <section v-if="loading" class="text-gray-400">Loading...</section>

      <section
        v-for="item in items"
        :key="item.id"
        class="bg-[#0a2540] rounded-lg p-4 flex items-center justify-between gap-4"
      >
        <div class="min-w-0">
          <p class="text-white truncate">{{ item.title || "Untitled" }}</p>
          <p class="text-xs text-gray-400">
            {{ item.type }} · deleted {{ formatDate(item.deleted_at) }}
          </p>
          <p v-if="item.persona_name" class="text-xs text-gray-500">
            Persona: {{ item.persona_name }}
          </p>
        </div>
        <div class="flex gap-2 shrink-0">
          <button
            class="bg-emerald-700 hover:bg-emerald-600 disabled:bg-gray-600 text-white px-3 py-1.5 rounded-lg text-sm"
            :disabled="busy"
            @click="restoreOne(item.id)"
          >
            Restore
          </button>
          <button
            v-if="isAdmin"
            class="bg-red-700 hover:bg-red-600 disabled:bg-gray-600 text-white px-3 py-1.5 rounded-lg text-sm"
            :disabled="busy"
            @click="purgeOne(item.id)"
          >
            Purge
          </button>
        </div>
      </section>

      <section v-if="!loading && items.length === 0" class="text-gray-400 text-sm">
        Trash is empty.
      </section>
    </main>
  </div>
</template>

<script setup>
import { onMounted, ref } from "vue";
import { getCurrentUser, listTrashItems, purgeItem, restoreItem } from "../services/api";

const items = ref([]);
const loading = ref(true);
const busy = ref(false);
const error = ref("");
const message = ref("");
const isAdmin = ref(false);
const scope = ref("mine");

function formatDate(value) {
  if (!value) return "unknown";
  return new Date(value).toLocaleString();
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await listTrashItems(scope.value);
    items.value = res.items || [];
  } catch (e) {
    error.value = e.message || "Failed to load trash";
  } finally {
    loading.value = false;
  }
}

async function restoreOne(itemId) {
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await restoreItem(itemId);
    message.value = "Item restored";
    await load();
  } catch (e) {
    error.value = e.message || "Failed to restore item";
  } finally {
    busy.value = false;
  }
}

async function purgeOne(itemId) {
  if (!window.confirm("Permanently purge this item? This cannot be undone.")) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    await purgeItem(itemId);
    message.value = "Item purged";
    await load();
  } catch (e) {
    error.value = e.message || "Failed to purge item";
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  const user = await getCurrentUser();
  isAdmin.value = user?.role === "admin";
  await load();
});
</script>
