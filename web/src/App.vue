<template>
  <div class="min-h-screen">
    <div
      v-if="showSessionActions"
      class="fixed top-3 right-3 z-50 flex items-center gap-2"
    >
      <router-link
        to="/me/profile"
        class="px-3 py-1.5 rounded-lg bg-[#0a2540] text-white text-sm hover:bg-[#12365d]"
      >
        Profile
      </router-link>
      <button
        :disabled="loggingOut"
        class="px-3 py-1.5 rounded-lg bg-red-700 text-white text-sm hover:bg-red-600 disabled:bg-gray-600"
        @click="handleLogout"
      >
        {{ loggingOut ? "Signing out..." : "Logout" }}
      </button>
    </div>
    <router-view />
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { getCurrentUser, logout } from "./services/api";

const route = useRoute();
const router = useRouter();

const user = ref(null);
const loggingOut = ref(false);

const showSessionActions = computed(
  () => !!user.value && route.name !== "login",
);

async function refreshUser() {
  user.value = await getCurrentUser();
}

async function handleLogout() {
  loggingOut.value = true;
  try {
    await logout();
  } finally {
    user.value = null;
    loggingOut.value = false;
    router.push({ name: "login" });
  }
}

watch(
  () => route.fullPath,
  async () => {
    await refreshUser();
  },
);

onMounted(async () => {
  await refreshUser();
});
</script>
