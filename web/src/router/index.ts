import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', component: () => import('@/views/HomeView.vue') },
    { path: '/chat/:id', component: () => import('@/views/ChatView.vue') },
    { path: '/skills', component: () => import('@/views/SkillsView.vue') },
    { path: '/agents', component: () => import('@/views/AgentsView.vue') },
  ]
})

export default router
