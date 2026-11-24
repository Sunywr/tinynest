<script setup lang="ts">
import { categoryIcon } from './categoryIcons'

withDefaults(defineProps<{ value: string; size?: number }>(), { size: 20 })
</script>

<template>
  <component :is="categoryIcon(value)"
    :size="size" :stroke-width="1.8" :data-icon="value" aria-hidden="true" />
</template>
