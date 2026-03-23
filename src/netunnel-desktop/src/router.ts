import { createRouter, createWebHashHistory } from "vue-router";
import BillingPage from "./pages/BillingPage.vue";
import DashboardPage from "./pages/DashboardPage.vue";
import TunnelsPage from "./pages/TunnelsPage.vue";
import UsagePage from "./pages/UsagePage.vue";
import SessionPage from "./pages/SessionPage.vue";

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: "/",
      redirect: "/dashboard",
    },
    {
      path: "/dashboard",
      component: DashboardPage,
    },
    {
      path: "/session",
      component: SessionPage,
    },
    {
      path: "/tunnels",
      component: TunnelsPage,
    },
    {
      path: "/billing",
      component: BillingPage,
    },
    {
      path: "/usage",
      component: UsagePage,
    },
  ],
});
