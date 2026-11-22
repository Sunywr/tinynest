<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppToast from './components/AppToast.vue'
import EntrySheet from './components/EntrySheet.vue'
import TabBar from './components/TabBar.vue'
import { useEntrySheet } from './composables/useEntrySheet'
import { useSession } from './composables/useSession'
import { APP_ROUTES } from './router'

const route = useRoute()
const router = useRouter()
const session = useSession()
const entry = useEntrySheet()
const { authed, booted } = session

const onPublicRoute = computed(() => APP_ROUTES.has(String(route.name)))

/** Public landing pages never render private data; protected pages need a session. */
function syncRoute() {
  if (!booted.value) return
  if (!authed.value && !onPublicRoute.value) router.replace('/')
  else if (authed.value && onPublicRoute.value) router.replace('/bills')
}

watch([authed, booted, () => route.name], syncRoute, { immediate: true })
watch(authed, signedIn => {
  if (!signedIn) entry.closeEntry()
})

let expiredTimer = 0
onMounted(() => {
  void session.boot().then(syncRoute)
  // A session can expire without a navigation; keep the URL honest.
  expiredTimer = window.setInterval(() => {
    if (authed.value && document.visibilityState === 'visible') void session.refreshMe().catch(() => undefined)
  }, 5 * 60 * 1000)
})
onBeforeUnmount(() => {
  window.clearInterval(expiredTimer)
  document.body.classList.remove('no-scroll')
})
</script>

<template>
  <div v-if="!booted" class="boot-splash" role="status" aria-live="polite">
    <img src="/icons/tinynest-original.png" alt="" />
    <p>正在打开小家…</p>
  </div>

  <router-view v-else-if="!authed" />

  <div v-else class="app-shell">
    <TabBar @entry="entry.openEntry()" />
    <main class="app-main">
      <router-view />
    </main>
  </div>

  <EntrySheet v-if="entry.open.value" :transaction="entry.editing.value" :preset-type="entry.presetType.value"
    @close="entry.closeEntry()" @saved="entry.closeEntry()" />
  <AppToast />
</template>

<style scoped>
.boot-splash {
  min-height: 100dvh;
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 10px;
  color: var(--muted);
  font-size: 13px;
}
.boot-splash img {
  width: 84px;
  height: 84px;
  object-fit: contain;
  border-radius: 24px;
}
.app-shell { min-height: 100dvh; background: var(--bg); }
.app-main {
  padding-bottom: calc(var(--tab-h) + env(safe-area-inset-bottom) + 16px);
  min-height: 100dvh;
}
@media (min-width: 1024px) {
  .app-main {
    margin-inline-start: var(--rail-w);
    padding-bottom: 24px;
    display: flex;
    justify-content: center;
  }
  .app-main > :deep(*) {
    width: 100%;
    max-width: 980px;
  }
}
</style>
