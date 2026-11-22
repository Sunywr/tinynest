<script setup lang="ts">
withDefaults(defineProps<{
  title: string
  hint?: string
  actionLabel?: string
  compact?: boolean
}>(), { hint: '', actionLabel: '', compact: false })

const emit = defineEmits<{ action: [] }>()
</script>

<template>
  <section class="empty-state" :class="{ compact }" role="status">
    <img src="/icons/tinynest-original.png" alt="" class="empty-mascot" />
    <h3>{{ title }}</h3>
    <p v-if="hint">{{ hint }}</p>
    <button v-if="actionLabel" type="button" class="btn-primary" @click="emit('action')">{{ actionLabel }}</button>
  </section>
</template>

<style scoped>
.empty-state {
  display: grid;
  justify-items: center;
  gap: 8px;
  padding: 36px 18px;
  text-align: center;
  color: var(--muted);
}
.empty-state.compact { padding: 20px 14px; }
.empty-mascot { width: 72px; height: 72px; object-fit: contain; border-radius: 22px; opacity: .96; }
.empty-state.compact .empty-mascot { width: 52px; height: 52px; }
.empty-state h3 { margin: 4px 0 0; color: var(--text); font-size: 15px; }
.empty-state p { margin: 0; font-size: 13px; line-height: 1.6; }
.empty-state .btn-primary { margin-top: 8px; }
</style>
