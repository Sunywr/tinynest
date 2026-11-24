<script setup lang="ts">
withDefaults(defineProps<{ name?: 'overlay' | 'flow' | 'popover' | 'view'; mode?: 'out-in' }>(), { name: 'overlay' })

function deactivate(element: Element) {
  element.setAttribute('inert', '')
}

function activate(element: Element) {
  element.removeAttribute('inert')
}
</script>

<template>
  <Transition :name="name" :mode="mode" @before-enter="activate" @before-leave="deactivate" @leave-cancelled="activate">
    <slot />
  </Transition>
</template>
