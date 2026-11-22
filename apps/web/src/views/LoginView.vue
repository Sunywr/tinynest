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
const registration = reactive({ email: '', code: '', invite: '' })
const challenge = ref('')
const cooldownUntil = ref(0)
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

async function sendRegistrationCode() {
  if (loading.value) return
  loading.value = true
  resetMessages()
  try {
    if (Date.now() < cooldownUntil.value) throw new Error('请在60秒后重发')
    const result = await api.mailChallenge({ email: registration.email, purpose: 'register' })
    challenge.value = result.challenge_id
    cooldownUntil.value = Date.now() + 60_000
    notice.value = '验证码已排队发送，10分钟内有效；同一邮箱每天最多3封邮件。'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '验证码发送失败'
  } finally {
    loading.value = false
  }
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
      notice.value = '申请已提交，管理员批准后即可登录本家庭。'
      form.password = ''
      return
    }
    if (!challenge.value) throw new Error('请先发送邮箱验证码')
    await api.register({
      username: form.username,
      password: form.password,
      email: registration.email,
      proof: { challenge_id: challenge.value, code: registration.code },
    })
    if (registration.invite.trim()) {
      await api.applyInvite({
        username: form.username,
        password: form.password,
        code: registration.invite.trim(),
      })
      notice.value = '注册与加入申请已提交，管理员批准后即可登录。'
    } else {
      notice.value = '注册请求已受理。拿到邀请码后可在登录页选择“用邀请码加入”。'
    }
    form.password = ''
    registration.code = ''
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
    notice.value = '邀请码已带入；完成注册后会提交加入申请。已注册但未加入的账号可选择“用邀请码加入”。'
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
      <p id="login-desc">{{ mode === 'join' ? '使用已注册但尚未加入本家庭的账号与邀请码提交申请。' : '登录后进入你的家庭账本。' }}</p>

      <PasswordRecovery v-if="mode === 'recover'" @close="switchMode('login')" />

      <form v-else @submit.prevent="submit">
        <label>用户名
          <input ref="usernameInput" v-model="form.username" autocomplete="username" required />
        </label>
        <label>密码
          <input v-model="form.password" type="password" autocomplete="current-password" required minlength="8" />
        </label>

        <template v-if="mode === 'register'">
          <p class="email-flow-hint">
            ① 填写邮箱并获取验证码 → ② 输入验证码和账号资料 → ③ 注册后凭邀请码申请加入家庭。
            同一邮箱每天最多3封邮件，上海时间零点重置。
          </p>
          <label>注册邮箱
            <input v-model="registration.email" type="email" required autocomplete="email" />
          </label>
          <button type="button" class="btn-quiet" :disabled="loading || !registration.email" @click="sendRegistrationCode">发送注册验证码</button>
          <label>邮箱验证码
            <input v-model="registration.code" inputmode="numeric" maxlength="6" required autocomplete="one-time-code" />
          </label>
          <label>邀请码（加入家庭时填写）
            <input v-model="registration.invite" autocomplete="off" />
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

        <button v-if="mode === 'login'" class="btn-quiet" type="button" @click="switchMode('recover')">忘记密码</button>
        <p v-if="error" class="err" role="alert">{{ error }}</p>
        <p v-if="notice" role="status">{{ notice }}</p>
        <button class="btn-primary" type="submit" :disabled="loading">
          {{ loading ? '处理中…' : mode === 'login' ? '登录' : mode === 'join' ? '提交加入申请' : registration.invite.trim() ? '注册并申请加入' : '注册' }}
        </button>
        <button v-if="mode === 'register'" class="btn-quiet" type="button" :disabled="loading" @click="switchMode('join')">
          已有账号？用邀请码加入
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
