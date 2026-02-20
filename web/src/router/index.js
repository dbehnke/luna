import { createRouter, createWebHistory } from "vue-router";
import LandingView from "../views/LandingView.vue";
import UploadView from "../views/UploadView.vue";
import LibraryView from "../views/LibraryView.vue";
import ItemView from "../views/ItemView.vue";
import ShortsView from "../views/ShortsView.vue";
import PersonasView from "../views/PersonasView.vue";
import ProfileView from "../views/ProfileView.vue";
import LoginView from "../views/LoginView.vue";
import { getCurrentUser } from "../services/api";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: "/login",
      name: "login",
      component: LoginView,
    },
    {
      path: "/",
      name: "home",
      component: LandingView,
    },
    {
      path: "/upload",
      name: "upload",
      component: UploadView,
    },
    {
      path: "/library",
      name: "library",
      component: LibraryView,
    },
    {
      path: "/item/:id",
      name: "item",
      component: ItemView,
    },
    {
      path: "/shorts",
      name: "shorts",
      component: ShortsView,
    },
    {
      path: "/personas",
      name: "personas",
      component: PersonasView,
    },
    {
      path: "/@:slug",
      name: "profile",
      component: ProfileView,
    },
  ],
});

router.beforeEach(async (to) => {
  const isLoginRoute = to.name === "login";
  const user = await getCurrentUser();

  if (!user && !isLoginRoute) {
    return { name: "login", query: { redirect: to.fullPath } };
  }

  if (user && isLoginRoute) {
    return { name: "home" };
  }

  return true;
});

export default router;
