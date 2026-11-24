<script setup lang="ts">
import { computed } from 'vue'
import { CreditCard, Landmark, TrendingUp, Wallet } from 'lucide-vue-next'
import { accountBrand, accountBrandArtwork } from '../services/accountBrand'

const props = defineProps<{ name: string; subtype?: string | null; kind?: string; markOnly?: boolean; logoKey?: string }>()
const brand = computed(() => props.logoKey || accountBrand(props.subtype, props.name, props.kind === 'huabei' ? 'alipay' : props.kind))
const artwork = computed(() => accountBrandArtwork(brand.value))
const fallbackIcon = computed(() => {
  if (props.kind === 'securities') return TrendingUp
  if (props.kind === 'wealth') return Wallet
  if (props.markOnly || /储蓄卡|借记卡|银行卡|工资卡/.test(props.name + (props.subtype ?? ''))) return CreditCard
  return Landmark
})
</script>

<template>
  <img v-if="artwork" class="account-icon" :class="{ 'card-mark': markOnly }" :src="`/account-brands/${artwork}.png`" :width="markOnly ? 48 : 24" :height="markOnly ? 48 : 24" alt="" />
  <span v-else class="account-icon fallback" :class="{ 'card-mark': markOnly }" aria-hidden="true"><component :is="fallbackIcon" :size="markOnly ? 24 : 16" /></span>
</template>

<style scoped>
.account-icon {
  display: inline-grid;
  width: 24px;
  height: 24px;
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
