<template>
  <div class="min-h-screen bg-[#07182c] flex flex-col items-center justify-center px-6">
    <main class="w-full max-w-3xl text-center">
      <img
        :src="lunaLogo"
        alt="Luna"
        class="w-full max-w-md mx-auto mb-8"
      />
      <h1 class="text-4xl font-bold text-white mb-2">Luna</h1>
      <p class="text-xl text-gray-400 mb-6">Moments live here</p>
      <p class="text-gray-300">Use the left navigation to browse Shorts, Library, Uploads, and more.</p>
    </main>
    <footer class="p-4 text-center text-gray-500 text-sm">
      <p>Family media server</p>
    </footer>
  </div>
</template>

<script setup>
import lunaLogo from '/src/assets/brand/luna-logo-full.png'
</script>
