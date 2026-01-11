<template>
  <div class="login-page">
    <div class="content">
      <div class="login-card">
        <h2 class="login-title">DevSecOps 平台登录</h2>
        <p class="login-subtitle">请输入账号和密码</p>

        <a-form layout="vertical" @submit.prevent="handleSubmit" :model="form">
          <a-form-item label="用户名">
            <a-input
              v-model:value="form.username"
              placeholder="请输入用户名"
              autocomplete="username"
            />
          </a-form-item>

          <a-form-item label="密码">
            <a-input-password
              v-model:value="form.password"
              placeholder="请输入密码"
              autocomplete="current-password"
            />
          </a-form-item>

          <a-form-item>
            <a-button
              type="primary"
              block
              :loading="loading"
              @click="handleSubmit"
            >
              登录
            </a-button>
          </a-form-item>
        </a-form>
      </div>
    </div>

    <div class="footer">DevSecOps-platform ©2025 Created by Yuy0ung</div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { useRouter } from "vue-router";
import request from "@/utils/request";
import { message } from "ant-design-vue";

const router = useRouter();

const form = reactive({
  username: "",
  password: "",
});

const loading = ref(false);

const handleSubmit = async () => {
  if (!form.username || !form.password) {
    message.warning("请输入用户名和密码");
    return;
  }

  loading.value = true;
  try {
    const res = await request.post("/api/login", {
      username: form.username,
      password: form.password,
    });

    const data = res.data || {};
    if (!data.token) {
      message.error("登录响应中缺少 token");
      return;
    }

    // 存储 token
    localStorage.setItem("token", data.token);
    localStorage.setItem("username", data.username || form.username);

    message.success("登录成功");

    // 登录成功后跳转
    router.push("/");
  } catch (e: any) {
    const msg =
      e?.response?.data?.error || e?.message || "登录失败，请检查用户名或密码";
    message.error(msg);
  } finally {
    loading.value = false;
  }
};
</script>

<style scoped>
/* 整页为垂直列布局，content 会撑开剩余高度，footer 自然在底部 */
.login-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column; /* 垂直排列：content 在上，footer 在下 */
  background: #f0f2f5; /* 浅灰背景，类似 Ant Design Pro */
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
    "Helvetica Neue", Arial, sans-serif;
}

.content {
  flex: 1; /* 占据剩余空间 */
  display: flex;
  justify-content: center;
  align-items: center; /* 居中显示登录卡片 */
  padding: 40px 20px;
}

.login-card {
  width: 100%;
  max-width: 400px;
  background: #fff;
  padding: 40px;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.login-title {
  margin: 0 0 8px;
  text-align: center;
  font-size: 24px;
  font-weight: 600;
  color: #1f2937;
}

.login-subtitle {
  margin: 0 0 32px;
  text-align: center;
  color: #6b7280;
  font-size: 14px;
}

.footer {
  text-align: center;
  padding: 24px 0;
  color: #9ca3af; /* 灰色文字 */
  font-size: 14px;
}
</style>
