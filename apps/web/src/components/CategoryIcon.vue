<script setup lang="ts">
import {
  Backpack, Bike, BriefcaseBusiness, Car, Cat, CircleDollarSign, Cloud, Clover,
  Coffee, CupSoda, CakeSlice, Croissant, Flower2, Gamepad2, Gift, House, LibraryBig,
  Mail, Palette, Pill, Rabbit, Ribbon, ShoppingBasket, ShoppingBag,
  Sandwich, Sprout, Star, ToyBrick, TrainFront, Trophy, Wallet, Cherry, Smile,
} from 'lucide-vue-next'

defineProps<{ value: string }>()

const icons = new Map([
  ['🍓', Cherry], ['🍙', Sandwich], ['🧁', CakeSlice], ['🧋', CupSoda], ['🍵', Coffee],
  ['🥐', Croissant], ['🐰', Rabbit], ['🐻', Smile], ['🐱', Cat], ['🌷', Flower2],
  ['🌱', Sprout], ['☁', Cloud], ['🏠', House], ['🧺', ShoppingBasket], ['🧸', ToyBrick],
  ['🎀', Ribbon], ['🎁', Gift], ['🛍', ShoppingBag], ['🚲', Bike], ['🚃', TrainFront],
  ['🏸', Trophy], ['📚', LibraryBig], ['🎨', Palette], ['💊', Pill], ['💌', Mail],
  ['🪙', CircleDollarSign], ['💰', Wallet], ['💼', BriefcaseBusiness], ['⭐', Star],
  ['🍀', Clover], ['🎮', Gamepad2], ['🧳', Backpack], ['🚗', Car],
])
</script>

<template>
  <component :is="icons.get(value.replace(/\uFE0F/g, '')) ?? Star"
    :size="20" :stroke-width="1.8" :data-icon="value" aria-hidden="true" />
</template>
