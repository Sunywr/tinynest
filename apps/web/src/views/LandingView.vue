<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ClipboardList, PenLine, PieChart, ShieldCheck, Users, Wallet } from 'lucide-vue-next'
import LoginView from './LoginView.vue'
import { useSession } from '../composables/useSession'

const route = useRoute()
const router = useRouter()
const session = useSession()
const ctaRef = ref<HTMLButtonElement | null>(null)

const loginOpen = computed(() => route.path === '/login')
const expired = computed(() => session.sessionExpired.value)

// Only capabilities backed by the shipped bookkeeping module are labelled 已支持.
// Credit statements, instalments, investments and statement imports were retired.
const features = [
  { icon: PenLine, title: '日常记账', desc: '支出、收入与账户转账；分类图标网格加自定义数字键盘，几秒记一笔。' },
  { icon: ClipboardList, title: '按天账单', desc: '按天分组与日小计，备注 / 分类搜索，日历按日筛选，点行更正、长按或菜单删除。' },
  { icon: PieChart, title: '图表分析', desc: '环形图看占比，分类排行按金额降序，支持月 / 年与支出 / 收入切换。' },
  { icon: Wallet, title: '资产与转账', desc: '现金与信用卡两类账户、净资产汇总、账户之间行内转账，余额由流水自动计算。' },
  { icon: Users, title: '家庭与预算', desc: '家庭成员标记、月度预算进度，管理员还可以邀请成员、审批申请并查看审计日志。' },
  { icon: ShieldCheck, title: '数据自主', desc: '数据库保存在你自己的服务器；HttpOnly 会话、CSRF 校验与按家庭隔离的查询。' },
]

function openLogin() {
  if (!loginOpen.value) void router.push('/login')
}
function closeLogin() {
  if (loginOpen.value) void router.replace('/')
  void nextTick(() => ctaRef.value?.focus())
}
function scrollToFeatures() {
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  document.getElementById('features')?.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'start' })
}
</script>

<template>
  <div class="landing-stage">
    <div class="landing">
      <header class="landing-header">
        <div class="landing-brand">
          <img src="/icons/tinynest-original.png" alt="TinyNest 原始糯米团子图标" class="landing-brand-icon" />
          <div><strong>TinyNest</strong><span>小家 · 家庭账本</span></div>
        </div>
        <nav class="landing-nav" aria-label="公开导航">
          <a href="#features" @click.prevent="scrollToFeatures()">功能</a>
          <button type="button" class="landing-login-link" @click="openLogin()">登录</button>
        </nav>
      </header>

      <main class="landing-main">
        <section class="hero" aria-labelledby="hero-title">
          <div class="hero-copy">
            <p class="hero-eyebrow">为家庭而设计 · 你的数据始终由你掌控</p>
            <h1 id="hero-title">家的每一笔，都值得被温柔记录。</h1>
            <p class="hero-sub">
              一个手机优先的家庭账本：按天分组的流水、一眼看懂的图表、诚实标注的资产账户，
              配色是熟悉的奶油团子。
            </p>
            <div class="hero-actions">
              <button ref="ctaRef" type="button" class="btn-primary hero-cta" @click="openLogin()">进入我的小家</button>
              <button type="button" class="btn-quiet" @click="scrollToFeatures()">了解功能 ↓</button>
            </div>
            <p v-if="expired" class="landing-notice" role="status">登录状态已过期，请重新进入。私人数据不会在此页显示。</p>
          </div>
          <div class="hero-visual" aria-hidden="true">
            <div class="mochi-glow"></div>
            <div class="mochi-float"><img src="/icons/tinynest-original.png" alt="" /></div>
          </div>
        </section>

        <section id="features" class="features" aria-labelledby="features-title">
          <div class="section-head">
            <h2 id="features-title">现在就能帮你做的事</h2>
          </div>
          <div class="feature-grid">
            <article v-for="feature in features" :key="feature.title" class="feature-card">
              <span class="feature-glyph" aria-hidden="true"><component :is="feature.icon" :size="20" /></span>
              <h3>{{ feature.title }}</h3>
              <p>{{ feature.desc }}</p>
              <span class="feature-status">已支持</span>
            </article>
          </div>
        </section>

        <section class="how" aria-labelledby="how-title">
          <h2 id="how-title">简单、透明、自己掌握</h2>
          <div class="how-grid">
            <div><strong>账目可核对</strong><p>每笔收支和转账都会生成分录，账户余额由流水自动计算，错账用冲正而不是改写历史。</p></div>
            <div><strong>数据自主</strong><p>数据库保存在你自己的服务器上，只请求同源 /api 相对路径，不接入任何第三方统计。</p></div>
            <div><strong>会话保护</strong><p>HttpOnly 会话 Cookie、CSRF 校验、登录失败限流，家庭数据在 SQL 层按家庭隔离。</p></div>
          </div>
        </section>
      </main>

      <footer class="landing-footer">
        <div class="footer-brand"><img src="/icons/tinynest-original.png" alt="" /> <span>TinyNest · 小家</span></div>
        <p>家庭账本 · V2 记账模块重建中，发布门禁尚未全部通过</p>
        <a href="https://github.com/Sunywr/tinynest" target="_blank" rel="noopener noreferrer">GitHub 仓库 ↗</a>
      </footer>
    </div>

    <LoginView v-if="loginOpen" @close="closeLogin" />
  </div>
</template>
