<template>
  <div class="min-h-screen bg-[#07182c]">
    <router-link
      v-if="showGlobalNav"
      to="/"
      class="md:hidden fixed top-3 left-3 z-[70] w-10 h-10 rounded-lg bg-[#0a2540] flex items-center justify-center"
      aria-label="Go to home"
    >
      <img :src="lunaMark" alt="Luna" class="w-7 h-7" />
    </router-link>

    <button
      v-if="showGlobalNav"
      class="md:hidden fixed top-3 right-3 z-[70] w-10 h-10 rounded-lg bg-[#0a2540] text-white flex items-center justify-center"
      @click="mobileNavOpen = true"
      aria-label="Open navigation menu"
    >
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
      </svg>
    </button>

    <aside
      v-if="showGlobalNav"
      class="hidden md:flex fixed left-0 top-0 bottom-0 z-40 bg-[#05152a] border-r border-[#123255] flex-col py-4 transition-all duration-200"
      :class="sidebarPinned ? 'w-56 px-3 items-stretch' : 'w-20 items-center'"
    >
      <div class="w-full flex items-center" :class="sidebarPinned ? 'justify-between gap-2 px-1 mb-3' : 'justify-center mb-4'">
        <router-link
          to="/"
          class="h-12 rounded-xl bg-[#0a2540] text-white flex items-center"
          :class="sidebarPinned ? 'px-3 gap-2 flex-1' : 'w-12 justify-center'"
          title="Home"
        >
          <svg class="w-6 h-6 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l9-9 9 9M5 10v10h14V10" />
          </svg>
          <span v-if="sidebarPinned" class="font-semibold">Luna</span>
        </router-link>
        <button
          class="h-12 rounded-xl bg-[#0a2540] text-gray-200 hover:bg-[#12365d] flex items-center justify-center"
          :class="sidebarPinned ? 'w-12' : 'hidden'"
          @click="toggleSidebarPin"
          title="Collapse sidebar"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <button
          class="h-12 w-12 rounded-xl bg-[#0a2540] text-gray-200 hover:bg-[#12365d] items-center justify-center"
          :class="sidebarPinned ? 'hidden' : 'flex'"
          @click="toggleSidebarPin"
          title="Expand sidebar"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
          </svg>
        </button>
      </div>

      <nav class="flex-1 flex flex-col gap-2 w-full" :class="sidebarPinned ? 'items-stretch' : 'items-center'">
        <router-link
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="rounded-xl flex items-center transition-colors"
          :class="[
            isRouteActive(item.to) ? 'bg-blue-600 text-white' : 'bg-[#0a2540] text-gray-200 hover:bg-[#12365d]',
            sidebarPinned ? 'h-11 px-3 gap-3 w-full' : 'w-12 h-12 justify-center'
          ]"
          :title="item.label"
        >
          <svg class="w-6 h-6 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" :d="item.icon" />
          </svg>
          <span v-if="sidebarPinned" class="text-sm font-medium">{{ item.label }}</span>
        </router-link>
      </nav>

      <button
        :disabled="loggingOut"
        class="rounded-xl bg-red-700 text-white hover:bg-red-600 disabled:bg-gray-600 flex items-center"
        :class="sidebarPinned ? 'h-11 px-3 gap-3 justify-start w-full' : 'w-12 h-12 justify-center'"
        @click="handleLogout"
        title="Logout"
      >
        <svg class="w-6 h-6 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H9m4 8H5a2 2 0 01-2-2V6a2 2 0 012-2h8" />
        </svg>
        <span v-if="sidebarPinned" class="text-sm font-medium">{{ loggingOut ? "Signing out..." : "Logout" }}</span>
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

    <div :class="desktopContentOffsetClass">
      <router-view />
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { getCurrentUser, logout } from "./services/api";
import lunaMark from "/src/assets/brand/luna-mark.png";

const route = useRoute();
const router = useRouter();

const user = ref(null);
const loggingOut = ref(false);
const mobileNavOpen = ref(false);
const sidebarPinned = ref(true);
const SIDEBAR_PIN_STORAGE_KEY = "luna_sidebar_pinned";

const showGlobalNav = computed(() => !!user.value && route.name !== "login");
const desktopContentOffsetClass = computed(() => {
  if (!showGlobalNav.value) return "";
  return sidebarPinned.value ? "md:pl-56" : "md:pl-20";
});
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

function toggleSidebarPin() {
  sidebarPinned.value = !sidebarPinned.value;
  if (typeof window !== "undefined") {
    window.localStorage.setItem(
      SIDEBAR_PIN_STORAGE_KEY,
      sidebarPinned.value ? "1" : "0",
    );
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
  if (typeof window !== "undefined") {
    const persisted = window.localStorage.getItem(SIDEBAR_PIN_STORAGE_KEY);
    if (persisted === "0") {
      sidebarPinned.value = false;
    }
  }
  await refreshUser();
});
</script>
