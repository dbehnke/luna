<template>
  <div class="min-h-screen bg-[#07182c]">
    <button
      v-if="showGlobalNav"
      class="md:hidden fixed top-3 left-3 z-[70] w-10 h-10 rounded-lg bg-[#0a2540] text-white flex items-center justify-center"
      @click="mobileNavOpen = true"
      aria-label="Open navigation menu"
    >
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
      </svg>
    </button>

    <aside
      v-if="showGlobalNav"
      class="hidden md:flex fixed left-0 top-0 bottom-0 z-40 w-20 bg-[#05152a] border-r border-[#123255] flex-col items-center py-4"
    >
      <router-link to="/" class="w-12 h-12 rounded-xl bg-[#0a2540] flex items-center justify-center text-white mb-5" title="Home">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l9-9 9 9M5 10v10h14V10" />
        </svg>
      </router-link>

      <nav class="flex-1 flex flex-col items-center gap-3">
        <router-link
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="w-12 h-12 rounded-xl flex items-center justify-center transition-colors"
          :class="isRouteActive(item.to) ? 'bg-blue-600 text-white' : 'bg-[#0a2540] text-gray-200 hover:bg-[#12365d]'"
          :title="item.label"
        >
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" :d="item.icon" />
          </svg>
        </router-link>
      </nav>

      <button
        :disabled="loggingOut"
        class="w-12 h-12 rounded-xl bg-red-700 text-white hover:bg-red-600 disabled:bg-gray-600 flex items-center justify-center"
        @click="handleLogout"
        title="Logout"
      >
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H9m4 8H5a2 2 0 01-2-2V6a2 2 0 012-2h8" />
        </svg>
      </button>
    </aside>

    <div
      v-if="showGlobalNav && mobileNavOpen"
      class="md:hidden fixed inset-0 z-[65] bg-black/60"
      @click="mobileNavOpen = false"
    ></div>

    <aside
      v-if="showGlobalNav"
      class="md:hidden fixed top-0 left-0 bottom-0 z-[70] w-72 bg-[#05152a] border-r border-[#123255] transform transition-transform duration-200"
      :class="mobileNavOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <div class="p-4 border-b border-[#123255] flex items-center justify-between">
        <span class="text-white font-semibold">Navigate</span>
        <button class="text-gray-300 hover:text-white" @click="mobileNavOpen = false" aria-label="Close navigation menu">
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <nav class="p-3 flex flex-col gap-2">
        <router-link
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="flex items-center gap-3 px-3 py-2 rounded-lg"
          :class="isRouteActive(item.to) ? 'bg-blue-600 text-white' : 'text-gray-200 hover:bg-[#12365d]'"
          @click="mobileNavOpen = false"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" :d="item.icon" />
          </svg>
          <span>{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="p-3 mt-auto border-t border-[#123255]">
        <button
          :disabled="loggingOut"
          class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg bg-red-700 text-white hover:bg-red-600 disabled:bg-gray-600"
          @click="handleLogout"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H9m4 8H5a2 2 0 01-2-2V6a2 2 0 012-2h8" />
          </svg>
          <span>{{ loggingOut ? "Signing out..." : "Logout" }}</span>
        </button>
      </div>
    </aside>

    <div :class="showGlobalNav ? 'md:pl-20' : ''">
      <router-view />
    </div>
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
const mobileNavOpen = ref(false);

const showGlobalNav = computed(() => !!user.value && route.name !== "login");
const navItems = computed(() => {
  const items = [
    { to: "/shorts", label: "Shorts", icon: "M8 5v14l11-7z" },
    { to: "/library", label: "Library", icon: "M4 6h16M4 12h16M4 18h16" },
    { to: "/upload", label: "Upload", icon: "M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 4v12m0-12l-4 4m4-4l4 4" },
    { to: "/me/profile", label: "Profile", icon: "M5.121 17.804A9 9 0 1118.88 17.8M15 11a3 3 0 11-6 0 3 3 0 016 0z" },
    { to: "/playlists", label: "Playlists", icon: "M4 6h16M4 12h16M4 18h10" },
    { to: "/trash", label: "Trash", icon: "M6 7h12M9 7V5h6v2m-7 4v6m4-6v6m4-6v6M5 7l1 12h12l1-12" },
  ];

  if (user.value?.role === "admin") {
    items.push({ to: "/admin/users", label: "Admin", icon: "M12 14l9-5-9-5-9 5 9 5zm0 0v6m-4-2h8" });
  }
  return items;
});

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

function isRouteActive(targetPath) {
  if (route.path === targetPath) return true;
  if (targetPath === "/library" && route.path.startsWith("/item/")) return true;
  return false;
}

watch(
  () => route.fullPath,
  async () => {
    mobileNavOpen.value = false;
    await refreshUser();
  },
);

onMounted(async () => {
  await refreshUser();
});
</script>
