import { createRouter, createWebHistory } from "vue-router";
import { getCurrentUser } from "../services/api";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: "/login",
      name: "login",
      component: () => import("../views/LoginView.vue"),
    },
    {
      path: "/",
      name: "home",
      component: () => import("../views/LandingView.vue"),
    },
    {
      path: "/upload",
      name: "upload",
      component: () => import("../views/UploadView.vue"),
    },
    {
      path: "/library",
      name: "library",
      component: () => import("../views/LibraryView.vue"),
    },
    {
      path: "/item/:id",
      name: "item",
      component: () => import("../views/ItemView.vue"),
    },
    {
      path: "/shorts",
      name: "shorts",
      component: () => import("../views/ShortsView.vue"),
    },
    {
      path: "/personas",
      name: "personas",
      redirect: "/me/profile",
    },
    {
      path: "/me/profile",
      name: "account-profile",
      component: () => import("../views/AccountView.vue"),
    },
    {
      path: "/@:slug",
      name: "profile",
      component: () => import("../views/ProfileView.vue"),
    },
    {
      path: "/admin/users",
      name: "admin-users",
      component: () => import("../views/AdminUsersView.vue"),
      meta: { requiresAdmin: true },
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

  if (to.matched.some((record) => record.meta?.requiresAdmin)) {
    if (!user || user.role !== "admin") {
      return { name: "home" };
    }
  }

  return true;
});

export default router;
