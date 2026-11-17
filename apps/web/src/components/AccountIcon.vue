<script setup lang="ts">
import { computed } from 'vue'
import { accountBrand } from '../services/accountBrand'

const props = defineProps<{ name: string; subtype?: string | null; kind?: string }>()
const brand = computed(() => accountBrand(props.subtype, props.name, props.kind === 'huabei' ? 'alipay' : props.kind))
</script>

<template>
  <img v-if="brand" class="account-icon" :src="`/account-brands/${brand}.png`" width="24" height="24" alt="" />
  <span v-else class="account-icon fallback" aria-hidden="true">{{ kind === 'securities' ? '↗' : kind === 'wealth' ? '◈' : '▤' }}</span>
</template>

<style scoped>
.account-icon {
  display: inline-grid;
  width: 24px;
  height: 24px;
  margin-inline-end: 7px;
  border-radius: 5px;
  vertical-align: middle;
  object-fit: contain;
}
.fallback { place-items: center; background: #f7eee3; color: #76503b; }
</style>
