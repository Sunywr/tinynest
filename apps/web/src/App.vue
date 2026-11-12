<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { checkApiHealth } from './services/api'

const displayDate = computed(() => `${new Date().getFullYear()} 年 ${new Date().getMonth() + 1} 月`)
const apiStatus = ref('检测中')
const notice = ref('')
let noticeTimer: ReturnType<typeof setTimeout> | undefined
const aborter = new AbortController()
function showDemoNotice() {
  notice.value = '这是初始化 Demo，实际记账功能尚未开发 ♡'
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => { notice.value = '' }, 2400)
}
onMounted(async () => {
  const timer = setTimeout(() => aborter.abort(), 4500)
  const ok = await checkApiHealth(aborter.signal)
  clearTimeout(timer)
  apiStatus.value = ok ? '已连接（健康检查）' : '未连接（不影响演示）'
})
onUnmounted(() => { aborter.abort(); if (noticeTimer) clearTimeout(noticeTimer) })
</script>
<template>
  <div class="stage">
<div class="website">
<aside class="sidebar">
<div class="brand"><img alt="TinyNest 原始糯米团子图标" class="logo" src="/icons/tinynest-original.png"/><div><div class="name">TinyNest</div><div class="desc">小家 · 生活有账，心里有光</div></div></div>
<div><div class="navlabel">我的小家</div><nav class="nav"><button class="active"><span class="glyph">⌂</span> 首页概览</button><button @click="showDemoNotice()"><span class="glyph">▤</span> 日常记账</button><button @click="showDemoNotice()"><span class="glyph">◉</span> 家庭账户</button><button @click="showDemoNotice()"><span class="glyph">◷</span> 预算计划</button><button @click="showDemoNotice()"><span class="glyph">▥</span> 收支报表</button><button @click="showDemoNotice()"><span class="glyph">⚙</span> 设置</button></nav></div>
<div class="sidebottom"><img alt="小家团子" class="blob logo" src="/icons/tinynest-original.png"/><p>每天一点点，攒满小幸福</p><small>从认真记录一餐一饭开始，<br/>让每个平凡日子都闪闪发光。</small></div>
</aside>
<div class="main">
<div class="mobilehead"><img alt="小家图标" class="logo" src="/icons/tinynest-original.png"/> TinyNest · 小家</div>
<div class="top"><span class="crumb">我的小家 / 首页概览</span><div class="topright"><span class="demotag">✦ 全部为虚构演示数据</span><span class="api-indicator" role="status">API：{{ apiStatus }}</span><span class="avatar">☺</span></div></div>
<section class="greeting"><div><h1>今天也要好好生活呀 ♡</h1><p>每一笔小小的记录，都在帮我们照顾这个温暖的家。</p></div><div class="day">{{ displayDate }}</div></section>
<div class="stats">
<div class="stat"><div class="stathead"><span class="statlabel">本月收入</span><span class="statglyph">☀</span></div><div class="statamount"><span class="unit">¥</span>24,800</div><div class="statfoot">一起努力的小小收获 ✧</div></div>
<div class="stat"><div class="stathead"><span class="statlabel">本月支出</span><span class="statglyph">☕</span></div><div class="statamount"><span class="unit">¥</span>8,620</div><div class="statfoot">认真生活的每个瞬间</div></div>
<div class="stat"><div class="stathead"><span class="statlabel">本月结余</span><span class="statglyph">♡</span></div><div class="statamount"><span class="unit">¥</span>16,180</div><div class="statfoot">把日子慢慢过成喜欢的样子</div></div>
</div>
<div class="content">
<div>
<section class="panel"><div class="paneltitle"><div><h2>收支小趋势</h2><span class="subinfo">近六个月的生活节奏</span></div><button @click="showDemoNotice()" class="switch">近 6 个月 ﹀</button></div>
<div aria-label="模拟收入支出条形图" class="graph">
<div class="barcol"><div class="barpair"><div class="bar income" style="height:75%"></div><div class="bar expense" style="height:40%"></div></div><div class="month">6月</div></div>
<div class="barcol"><div class="barpair"><div class="bar income" style="height:81%"></div><div class="bar expense" style="height:49%"></div></div><div class="month">7月</div></div>
<div class="barcol"><div class="barpair"><div class="bar income" style="height:67%"></div><div class="bar expense" style="height:36%"></div></div><div class="month">8月</div></div>
<div class="barcol"><div class="barpair"><div class="bar income" style="height:88%"></div><div class="bar expense" style="height:61%"></div></div><div class="month">9月</div></div>
<div class="barcol"><div class="barpair"><div class="bar income" style="height:78%"></div><div class="bar expense" style="height:52%"></div></div><div class="month">10月</div></div>
<div class="barcol"><div class="barpair"><div class="bar income" style="height:92%"></div><div class="bar expense" style="height:46%"></div></div><div class="month">11月</div></div>
</div>
<div class="legend"><span><i class="key" style="background:var(--bar)"></i>收入</span><span><i class="key" style="background:var(--primary)"></i>支出</span></div>
</section>
<section class="panel transactions"><div class="paneltitle"><div><h2>最近的小账本</h2><span class="subinfo">那些认真生活的小瞬间</span></div><button @click="showDemoNotice()" class="switch">查看全部 →</button></div>
<div class="translist">
<div class="transaction"><div class="txicon">🥐</div><div class="txbody"><div class="txname">早餐时光</div><div class="txcat">餐饮 · 今天</div></div><div class="txamount">− ¥28.00</div></div>
<div class="transaction"><div class="txicon">🛒</div><div class="txbody"><div class="txname">周末超市</div><div class="txcat">生活购物 · 昨天</div></div><div class="txamount">− ¥186.50</div></div>
<div class="transaction"><div class="txicon">🚇</div><div class="txbody"><div class="txname">通勤交通</div><div class="txcat">出行 · 11月10日</div></div><div class="txamount">− ¥6.00</div></div>
<div class="transaction"><div class="txicon">🌼</div><div class="txbody"><div class="txname">月度收入</div><div class="txcat">工资收入 · 11月8日</div></div><div class="txamount in">+ ¥24,800</div></div>
</div>
</section>
</div>
<div><section class="panel budget"><div class="paneltitle"><div><h2>本月预算</h2><span class="subinfo">慢慢花，也能开心存</span></div><span style="font-size:19px">🌷</span></div><div class="budgetrow"><b>¥ 8,620 <span style="font-weight:400">/ ¥ 13,500</span></b><span>64%</span></div><div class="track"><div class="fill"></div></div><div class="budgetnote">还有 ¥ 4,880 的预算可以安排。<br/>日子不必紧绷，心里有数就好。</div><div class="note-box">♡ 小家碎碎念<br/>记录不是限制生活，而是让我们更自由地选择生活。</div></section><p class="legend-note">⋆｡°✩ 用轻轻柔柔的方式记下每一天 ✩°｡⋆</p></div>
</div>
</div>
</div>
    <div v-if="notice" class="toast show" role="status">{{ notice }}</div>
  </div>
</template>
