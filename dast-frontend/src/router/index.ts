import { createRouter, createWebHistory, RouteRecordRaw } from "vue-router";
import HomePage from "@/pages/HomePage.vue";
import DesigningPage from "@/pages/DesigningPage.vue";
import DastList from "@/pages/dast/DastList.vue";
import TargetList from "@/pages/dast/TargetList.vue";
import LoginPage from "@/pages/LoginPage.vue";

const routes: Array<RouteRecordRaw> = [
  {
    path: "/login",
    name: "login",
    component: LoginPage,
    meta: { hideLayout: true },
  },
  {
    path: "/",
    name: "home",
    component: HomePage,
    meta: { requiresAuth: true },
  },
  {
    path: "/dast/task",
    name: "dastTask",
    component: DastList,
    meta: { requiresAuth: true },
  },
  {
    path: "/dast/task/targets",
    name: "dastTarget",
    component: TargetList,
    meta: { requiresAuth: true },
  },
  {
    path: "/dast/poc",
    name: "dastPoc",
    component: DesigningPage,
    meta: { requiresAuth: true },
  },
  {
    path: "/sast/codeql",
    name: "sastCodeql",
    component: DesigningPage,
    meta: { requiresAuth: true },
  },
  {
    path: "/sast/vuln",
    name: "sastVuln",
    component: DesigningPage,
    meta: { requiresAuth: true },
  },
  {
    path: "/sca/list",
    name: "scaList",
    component: DesigningPage,
    meta: { requiresAuth: true },
  },
  {
    path: "/sca/vuln",
    name: "scaVuln",
    component: DesigningPage,
    meta: { requiresAuth: true },
  },
];

const router = createRouter({
  history: createWebHistory(process.env.BASE_URL),
  routes,
});

// 全局前置守卫
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem("token");

  //没有token跳到/login
  if (to.meta.requiresAuth && !token) {
    next({
      path: "/login",
      query: { redirect: to.fullPath }, // 可选：登录后可以跳回原页面
    });
    return;
  }

  if (to.path === "/login" && token) {
    next({ path: "/" });
    return;
  }

  next();
});

export default router;
