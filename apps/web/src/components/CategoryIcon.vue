<script setup lang="ts">
import { categoryIcon } from './categoryIcons'

defineProps<{ value: string }>()
</script>

<template>
  <component :is="categoryIcon(value)"
    :size="20" :stroke-width="1.8" :data-icon="value" aria-hidden="true" />
</template>
