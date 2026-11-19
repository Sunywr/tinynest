<script setup lang="ts">
import { computed } from 'vue'
import { Landmark, TrendingUp, Wallet } from 'lucide-vue-next'
import { accountBrand } from '../services/accountBrand'

const props = defineProps<{ name: string; subtype?: string | null; kind?: string; markOnly?: boolean }>()
const brand = computed(() => accountBrand(props.subtype, props.name, props.kind === 'huabei' ? 'alipay' : props.kind))
const artwork = computed(() => {
  if (!props.markOnly) return brand.value
  return brand.value === 'ceb' || brand.value === 'spdb' ? brand.value + '-mark' : null
})
</script>

<template>
  <img v-if="artwork" class="account-icon" :class="{ 'card-mark': markOnly }" :src="`/account-brands/${artwork}.png`" :width="markOnly ? 48 : 24" :height="markOnly ? 48 : 24" alt="" />
  <span v-else class="account-icon fallback" :class="{ 'card-mark': markOnly }" aria-hidden="true"><component :is="kind === 'securities' ? TrendingUp : kind === 'wealth' ? Wallet : Landmark" :size="markOnly ? 24 : 16" /></span>
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
.card-mark {
  flex: 0 0 48px;
  width: 48px;
  height: 48px;
  padding: 7px;
  margin: 0;
  border-radius: 12px;
  background: var(--soft);
}
</style>
