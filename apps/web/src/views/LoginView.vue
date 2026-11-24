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
const captcha = ref<{ id: string; image: string }>()
const captchaAnswer = ref('')
const captchaLoading = ref(false)
const captchaError = ref('')
let captchaRequest = 0

function needsCaptcha() {
  return mode.value === 'login' || mode.value === 'register'
}

async function refreshCaptcha() {
  const request = ++captchaRequest
  captcha.value = undefined
  captchaAnswer.value = ''
  captchaError.value = ''
  captchaLoading.value = needsCaptcha()
  if (!needsCaptcha()) return
  try {
    const challenge = await api.captcha()
    if (request === captchaRequest) captcha.value = challenge
  } catch (e) {
    if (request === captchaRequest) captchaError.value = e instanceof Error ? e.message : '验证码加载失败，请刷新'
  } finally {
    if (request === captchaRequest) captchaLoading.value = false
  }
}

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
  void refreshCaptcha()
  if (next === 'login') void nextTick(() => usernameInput.value?.focus())
}

async function submit() {
  if (loading.value) return
  loading.value = true
  resetMessages()
  try {
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
    if (!captcha.value || !/^\d{6}$/.test(captchaAnswer.value)) throw new Error('请输入图片中的 6 位验证码')
    const proof = { id: captcha.value.id, answer: captchaAnswer.value }
    if (mode.value === 'register') {
      await session.register(form.username, form.password, proof)
    } else {
      await session.login(form.username, form.password, proof)
    }
    form.password = ''
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败，请重试'
    if (needsCaptcha()) void refreshCaptcha()
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
  void refreshCaptcha()
  await nextTick()
  usernameInput.value?.focus()
})
onBeforeUnmount(() => {
  captchaRequest++
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

      <PasswordRecovery v-if="mode === 'recover'" class="flow-in" @close="switchMode('login')" />

      <form v-else :key="mode" class="flow-in" @submit.prevent="submit">
        <label>用户名
          <input ref="usernameInput" v-model="form.username" autocomplete="username" required />
        </label>
        <label>密码
          <input v-model="form.password" type="password" :autocomplete="mode === 'register' ? 'new-password' : 'current-password'" required minlength="8" maxlength="256" />
        </label>

        <template v-if="needsCaptcha()">
          <div class="captcha-row">
            <img v-if="captcha" :src="captcha.image" :data-challenge-id="captcha.id" alt="六位数字验证码" width="202" height="56" />
            <button type="button" class="btn-quiet" :disabled="loading || captchaLoading" @click="refreshCaptcha">
              {{ captchaLoading ? '加载中…' : '换一张验证码' }}
            </button>
          </div>
          <p v-if="captchaError" class="err" role="alert">{{ captchaError }}</p>
          <label>图形验证码
            <input v-model="captchaAnswer" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="off" required />
          </label>
        </template>

        <template v-if="mode === 'join'">
          <p class="email-flow-hint">
            适用于已注册、但尚未加入本家庭的账号。填写登录密码与管理员给的邀请码；批准后即可共享该家庭的账户与流水。
          </p>
          <label>邀请码
            <input v-model="registration.invite" required autocomplete="off" />
          </label>
        </template>

        <button v-if="mode === 'login'" class="btn-quiet" type="button" :disabled="loading" @click="switchMode('recover')">忘记密码</button>
        <p v-if="error" class="err" role="alert">{{ error }}</p>
        <p v-if="notice" role="status">{{ notice }}</p>
        <button class="btn-primary" type="submit" :disabled="loading || (needsCaptcha() && !captcha)">
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

<style scoped>
.captcha-row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.captcha-row img { max-width: 100%; height: auto; border-radius: 8px; }
.captcha-row .btn-quiet { width: auto; flex: 1 0 auto; }
</style>
