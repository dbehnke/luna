<template>
  <span
    v-if="displayName"
    :class="[
      'inline-flex items-center gap-1.5',
      variant === 'compact' ? 'text-xs' : 'text-sm'
    ]"
  >
    <span
      :class="[
        'rounded-full flex items-center justify-center font-medium',
        variant === 'compact' ? 'w-5 h-5 text-[10px]' : 'w-6 h-6 text-xs',
        'bg-purple-600 text-white'
      ]"
    >
      {{ initials }}
    </span>
    <span :class="variant === 'compact' ? 'text-gray-300' : 'text-gray-300'">
      {{ displayName }}
    </span>
  </span>
  <span
    v-else
    :class="[
      'inline-flex items-center gap-1.5',
      variant === 'compact' ? 'text-xs' : 'text-sm'
    ]"
  >
    <span
      :class="[
        'rounded-full flex items-center justify-center font-medium',
        variant === 'compact' ? 'w-5 h-5 text-[10px]' : 'w-6 h-6 text-xs',
        'bg-gray-600 text-white'
      ]"
    >
      ?
    </span>
    <span :class="variant === 'compact' ? 'text-gray-400' : 'text-gray-400'">
      Self
    </span>
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  displayName: {
    type: String,
    default: null
  },
  variant: {
    type: String,
    default: 'compact',
    validator: (value) => ['compact', 'overlay'].includes(value)
  }
})

const initials = computed(() => {
  if (!props.displayName) return '?'
  const words = props.displayName.trim().split(/\s+/)
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase()
  }
  return props.displayName.slice(0, 2).toUpperCase()
})
</script>
