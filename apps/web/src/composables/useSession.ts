/** Single source of truth for the signed-in session, household and members. */

import { computed, ref } from 'vue'
import { api, setCSRFToken, type CaptchaProof, type Household, type Me, type Member } from '../services/api'

const me = ref<Me | null>(null)
const household = ref<Household | null>(null)
const members = ref<Member[]>([])
const authed = ref(false)
const booted = ref(false)
const sessionExpired = ref(false)
const booting = ref(false)

const hasHousehold = computed(() => Boolean(me.value?.household_id))
const canWrite = computed(() => hasHousehold.value && me.value?.role !== 'viewer')
const isAdmin = computed(() => me.value?.role === 'admin')
const currentMember = computed(() => members.value.find(m => m.id === me.value?.member_id) ?? null)
const hasMultipleMembers = computed(() => members.value.length > 1)

function clearSession() {
  authed.value = false
  me.value = null
  household.value = null
  members.value = []
  setCSRFToken('')
}

async function loadContext() {
  const [h, m] = await Promise.all([api.household(), api.members()])
  household.value = h
  members.value = m.items
}

async function boot() {
  if (booting.value) return
  booting.value = true
  try {
    const profile = await api.me()
    setCSRFToken(profile.csrf_token || '')
    me.value = profile
    authed.value = true
    sessionExpired.value = false
    if (hasHousehold.value) await loadContext()
  } catch {
    clearSession()
  } finally {
    booting.value = false
    booted.value = true
  }
}

async function login(username: string, password: string, captcha: CaptchaProof) {
  await api.login(username, password, captcha)
  await loadSignedInSession()
}

async function register(username: string, password: string, captcha: CaptchaProof) {
  await api.register(username, password, captcha)
  await loadSignedInSession()
}

async function loadSignedInSession() {
  const profile = await api.me()
  setCSRFToken(profile.csrf_token || '')
  me.value = profile
  authed.value = true
  sessionExpired.value = false
  if (hasHousehold.value) await loadContext()
}

async function logout() {
  try {
    await api.logout()
  } catch {
    /* the cookie is cleared client-side regardless */
  }
  clearSession()
}

function expire() {
  sessionExpired.value = true
  clearSession()
}

async function refreshMe() {
  const profile = await api.me()
  setCSRFToken(profile.csrf_token || '')
  const joined = !hasHousehold.value && Boolean(profile.household_id)
  if (joined) await loadContext()
  if (!profile.household_id) {
    household.value = null
    members.value = []
  }
  me.value = profile
}

async function refreshHousehold() {
  await loadContext()
}

if (typeof window !== 'undefined') {
  window.addEventListener('tinynest:session-expired', () => {
    if (authed.value) expire()
  })
}

export function useSession() {
  return {
    me,
    household,
    members,
    authed,
    booted,
    booting,
    sessionExpired,
    canWrite,
    hasHousehold,
    isAdmin,
    currentMember,
    hasMultipleMembers,
    boot,
    login,
    register,
    logout,
    expire,
    refreshMe,
    refreshHousehold,
    clearSession,
  }
}
