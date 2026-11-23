<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import PasswordRecovery from '../components/PasswordRecovery.vue'
import { api } from '../services/api'
import { useSession } from '../composables/useSession'

const emit = defineEmits<{ close: [] }>()
const session = useSession()
const route = useRoute()

const mode = ref<'login' | 'register' | 'join' | 'recover'>('login')
const form = reactive({ username: '', password: '' })
const registration = reactive({ invite: '' })
const loading = ref(false)
const error = ref('')
const notice = ref('')

const dialog = ref<HTMLElement | null>(null)
const usernameInput = ref<HTMLInputElement | null>(null)

function resetMessages() {
  error.value = ''
  notice.value = ''
}

function switchMode(next: 'login' | 'register' | 'join' | 'recover') {
  mode.value = next
  resetMessages()
  form.password = ''
  if (next === 'login') void nextTick(() => usernameInput.value?.focus())
}

async function submit() {
  if (loading.value) return
  loading.value = true
  resetMessages()
  try {
    if (mode.value === 'login') {
      await session.login(form.username, form.password)
      form.password = ''
      return
    }
    if (mode.value === 'join') {
      if (!registration.invite.trim()) throw new Error('请填写邀请码')
      await api.applyInvite({
        username: form.username,
        password: form.password,
        code: registration.invite.trim(),
      })
      notice.value = '申请已提交，管理员批准后即可使用家庭账本。'
      form.password = ''
      return
    }
    await api.register({ username: form.username, password: form.password })
    mode.value = 'login'
    await session.login(form.username, form.password)
    form.password = ''
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败，请重试'
  } finally {
    loading.value = false
  }
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
    return
  }
  if (event.key !== 'Tab') return
  const focusables = Array.from(dialog.value?.querySelectorAll<HTMLElement>(
    'button:not([disabled]), [href], input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])',
  ) ?? []).filter(el => el.offsetParent !== null)
  if (!focusables.length) return
  const first = focusables[0]
  const last = focusables[focusables.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}

onMounted(async () => {
  document.body.classList.add('no-scroll')
  window.addEventListener('keydown', onKeydown)
  const invite = typeof route.query.invite === 'string' ? route.query.invite.trim() : ''
  if (invite) {
    mode.value = 'register'
    registration.invite = invite
    notice.value = '邀请码已带入。登录并验证邮箱后可提交加入申请。'
  }
  await nextTick()
  usernameInput.value?.focus()
})
onBeforeUnmount(() => {
  document.body.classList.remove('no-scroll')
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="dialog-backdrop" @click.self="emit('close')">
    <div ref="dialog" class="login-dialog" role="dialog" aria-modal="true" aria-labelledby="login-title" aria-describedby="login-desc">
      <button type="button" class="dialog-close" aria-label="关闭登录" @click="emit('close')">×</button>
      <img src="/icons/tinynest-original.png" alt="" class="dialog-logo" />
      <h2 id="login-title">{{ mode === 'join' ? '加入家庭' : '欢迎回家' }}</h2>
      <p id="login-desc">{{ mode === 'join' ? '使用已注册但尚未加入本家庭的账号与邀请码提交申请。' : mode === 'register' ? '注册后即可登录，验证邮箱后可创建或加入家庭。' : '登录后继续使用小家。' }}</p>

      <PasswordRecovery v-if="mode === 'recover'" @close="switchMode('login')" />

      <form v-else @submit.prevent="submit">
        <label>用户名
          <input ref="usernameInput" v-model="form.username" autocomplete="username" required />
        </label>
        <label>密码
          <input v-model="form.password" type="password" :autocomplete="mode === 'register' ? 'new-password' : 'current-password'" required :minlength="mode === 'register' ? 12 : 8" />
        </label>

        <template v-if="mode === 'join'">
          <p class="email-flow-hint">
            适用于已注册、但尚未加入本家庭的账号。填写登录密码与管理员给的邀请码；批准后即可共享该家庭的账户与流水。
          </p>
          <label>邀请码
            <input v-model="registration.invite" required autocomplete="off" />
          </label>
        </template>

        <button v-if="mode === 'login'" class="btn-quiet" type="button" @click="switchMode('recover')">忘记密码</button>
        <p v-if="error" class="err" role="alert">{{ error }}</p>
        <p v-if="notice" role="status">{{ notice }}</p>
        <button class="btn-primary" type="submit" :disabled="loading">
          {{ loading ? '处理中…' : mode === 'login' ? '登录' : mode === 'join' ? '提交加入申请' : '注册并登录' }}
        </button>
        <button v-if="mode === 'register'" class="btn-quiet" type="button" :disabled="loading" @click="switchMode('join')">
          旧版待审批账号？用邀请码加入
        </button>
        <button v-if="mode === 'join'" class="btn-quiet" type="button" :disabled="loading" @click="switchMode('register')">
          注册新账号
        </button>
        <button class="btn-quiet" type="button" :disabled="loading" @click="switchMode(mode === 'login' ? 'register' : 'login')">
          {{ mode === 'login' ? '注册新账号' : '返回登录' }}
        </button>
      </form>
    </div>
  </div>
</template>
