<script setup lang="ts">
import AccountIcon from './AccountIcon.vue'
import type { Account } from '../services/api'

defineProps<{ account: Account }>()
const emit = defineEmits<{ select: [] }>()
</script>

<template>
  <button type="button" class="account-row" :class="{ credit: account.kind === 'credit' }"
    :aria-label="account.name" :title="account.name" @click="emit('select')">
    <AccountIcon :name="account.name" :subtype="account.subtype" :kind="account.kind" aria-hidden="true" />
  </button>
</template>

<style scoped>
.account-row {
  display: grid;
  place-items: center;
  width: var(--touch-min);
  height: var(--touch-min);
  padding: 0;
  border: 0;
  border-radius: 14px;
  background: var(--cat-3);
  color: #5e6b53;
  cursor: pointer;
}
.account-row.credit { background: var(--cat-1); color: #8a5a4c; }
.account-row:hover:not(:disabled) { box-shadow: inset 0 0 0 1px currentColor; }
</style>
