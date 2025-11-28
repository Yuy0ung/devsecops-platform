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
  background-color: #ffffff; /* 整页白色背景 */
  box-sizing: border-box;
}

/* content 占据可用空间并居中显示登录卡 */
.content {
  flex: 1; /* 向下推 footer，使 footer 固定在页面底部（视觉上） */
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
}

.login-card {
  width: 360px;
  max-width: 100%;
  background-color: #ffffff;
  border-radius: 12px;
  padding: 24px 24px 16px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.3);
  box-sizing: border-box;
}

.login-title {
  margin: 0 0 4px;
  font-size: 22px;
  font-weight: 600;
  color: #111827;
  text-align: center;
}

.login-subtitle {
  margin: 0 0 24px;
  font-size: 13px;
  color: #6b7280;
  text-align: center;
}

.login-tip {
  margin-top: 8px;
  font-size: 12px;
  color: #9ca3af;
  text-align: center;
}

.login-tip code {
  background: #f3f4f6;
  padding: 2px 4px;
  border-radius: 4px;
  font-size: 12px;
}

/* 页脚样式：当 content 不够高时，此处会贴到底部；当内容很多时则在内容下方 */
.login-page .footer {
  text-align: center;
  font-size: 14px;
  padding: 12px 16px;
  color: #6b7280;
  background: #ffffff;
}
</style>
