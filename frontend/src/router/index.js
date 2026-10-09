import { createRouter, createWebHistory } from 'vue-router'

// Views are lazily imported: the reader is the only screen most sessions ever
// open, and it should not wait on the catalogue or the auth pages.
const routes = [
  { path: '/', name: 'home', component: () => import('../views/HomeView.vue') },
  { path: '/catalog', name: 'catalog', component: () => import('../views/CatalogView.vue') },
  { path: '/read/:bookId', name: 'read', component: () => import('../views/ReaderView.vue') },
  { path: '/search', name: 'search', component: () => import('../views/SearchView.vue') },
  { path: '/vocab', name: 'vocab', component: () => import('../views/VocabView.vue') },
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
  { path: '/register', name: 'register', component: () => import('../views/RegisterView.vue') },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export default createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from, saved) {
    if (saved) return saved
    return { top: 0 }
  },
})
