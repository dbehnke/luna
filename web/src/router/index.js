import { createRouter, createWebHistory } from 'vue-router'
import LandingView from '../views/LandingView.vue'
import UploadView from '../views/UploadView.vue'
import LibraryView from '../views/LibraryView.vue'
import ItemView from '../views/ItemView.vue'
import ShortsView from '../views/ShortsView.vue'
import PersonasView from '../views/PersonasView.vue'
import ProfileView from '../views/ProfileView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: LandingView
    },
    {
      path: '/upload',
      name: 'upload',
      component: UploadView
    },
    {
      path: '/library',
      name: 'library',
      component: LibraryView
    },
    {
      path: '/item/:id',
      name: 'item',
      component: ItemView
    },
    {
      path: '/shorts',
      name: 'shorts',
      component: ShortsView
    },
    {
      path: '/personas',
      name: 'personas',
      component: PersonasView
    },
    {
      path: '/@:slug',
      name: 'profile',
      component: ProfileView
    }
  ]
})

export default router
