import { createRouter, createWebHistory } from 'vue-router'
import BillsView from './views/BillsView.vue'
import ChartsView from './views/ChartsView.vue'
import AssetsView from './views/AssetsView.vue'
import MeView from './views/MeView.vue'
import SettingsView from './views/SettingsView.vue'
import LandingView from './views/LandingView.vue'

/** Vue Router 4 with normal (hash-free) history mode. Protected routes are
 *  guarded by App.vue against the session state so the public landing page can
 *  always render without a redirect loop. */
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'landing', component: LandingView, meta: { public: true } },
    { path: '/login', name: 'login', component: LandingView, meta: { public: true } },
    { path: '/bills', name: 'bills', component: BillsView, meta: { nav: '账单' } },
    { path: '/charts', name: 'charts', component: ChartsView, meta: { nav: '图表' } },
    { path: '/assets', name: 'assets', component: AssetsView, meta: { nav: '资产' } },
    { path: '/me', name: 'me', component: MeView, meta: { nav: '我的' } },
    { path: '/me/settings', name: 'settings', component: SettingsView, meta: { nav: '设置' } },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

export const APP_ROUTES = new Set(['landing', 'login'])
