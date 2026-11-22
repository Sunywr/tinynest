<script setup lang="ts">
import { computed } from 'vue'
import AccountIcon from './AccountIcon.vue'
import { formatMinor } from '../services/format'
import type { Account } from '../services/api'

const props = defineProps<{ account: Account; memberName?: string }>()
const emit = defineEmits<{ select: [] }>()

const isCredit = computed(() => props.account.kind === 'credit')
const balance = computed(() => {
  const raw = props.account.balance_minor ?? 0
  return isCredit.value ? -Math.abs(raw) : raw
})
const balanceText = computed(() => {
  if (!isCredit.value) return formatMinor(balance.value)
  return balance.value === 0 ? formatMinor(0) : `欠 ${formatMinor(Math.abs(balance.value))}`
})
</script>

<template>
  <button type="button" class="account-row" @click="emit('select')">
    <span class="account-glyph" :class="{ credit: isCredit }" aria-hidden="true">
      <AccountIcon :name="account.name" :subtype="account.subtype" :kind="account.kind" />
    </span>
    <span class="account-body">
      <span class="account-name">{{ account.name }}</span>
      <span class="account-sub">
        {{ isCredit ? '信用卡 · 负债账户' : '现金账户' }}
        <em v-if="memberName"> · {{ memberName }}</em>
        <em v-else-if="account.note"> · {{ account.note }}</em>
      </span>
    </span>
    <span class="account-balance" :class="{ negative: isCredit && balance !== 0 }">{{ balanceText }}</span>
  </button>
</template>

<style scoped>
.account-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: var(--touch-min);
  padding: 10px 2px;
  border: 0;
  border-bottom: 1px solid var(--border);
  background: transparent;
  text-align: left;
  color: var(--text);
}
.account-row:last-child { border-bottom: 0; }
.account-glyph {
  flex: none;
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 14px;
  background: var(--cat-3);
  color: #5e6b53;
}
.account-glyph.credit { background: var(--cat-1); color: #8a5a4c; }
.account-body { flex: 1; min-width: 0; display: grid; gap: 2px; }
.account-name { font-size: 14px; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.account-sub { font-size: 11px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.account-sub em { font-style: normal; }
.account-balance { font-size: 14px; font-weight: 750; font-variant-numeric: tabular-nums; white-space: nowrap; }
.account-balance.negative { color: #a04e43; }
</style>
