<template>
  <div class="codeql-scan-container">
    <a-card title="CodeQL SAST 扫描" :bordered="false">
      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="git" tab="Git仓库扫描">
          <a-form layout="vertical" :model="gitForm">
            <a-form-item label="Repository URL" required>
              <a-input
                v-model:value="gitForm.repoUrl"
                placeholder="https://github.com/owner/repo.git"
              />
            </a-form-item>
            <a-form-item label="Branch">
              <a-input v-model:value="gitForm.branch" placeholder="main" />
            </a-form-item>
            <a-form-item label="Rules (CWE)" required>
              <a-checkbox-group
                v-model:value="gitForm.rules"
                style="width: 100%"
              >
                <a-row>
                  <a-col
                    :span="8"
                    v-for="rule in ruleOptions"
                    :key="rule.value"
                  >
                    <a-checkbox :value="rule.value">{{
                      rule.label
                    }}</a-checkbox>
                  </a-col>
                </a-row>
              </a-checkbox-group>
            </a-form-item>
            <a-form-item>
              <a-button type="primary" :loading="loading" @click="handleGitScan"
                >Start Scan</a-button
              >
            </a-form-item>
          </a-form>
        </a-tab-pane>

        <a-tab-pane key="upload" tab="上传CodeQL数据库扫描">
          <a-upload-dragger
            v-model:fileList="fileList"
            name="file"
            :multiple="false"
            accept=".zip"
            :before-upload="beforeUpload"
            @remove="handleRemove"
          >
            <p class="ant-upload-drag-icon">
              <inbox-outlined />
            </p>
            <p class="ant-upload-text">
              Click or drag file to this area to upload
            </p>
            <p class="ant-upload-hint">
              Support for a single or bulk upload. Strictly prohibit from
              uploading company data or other band files
            </p>
          </a-upload-dragger>
          <div
            style="margin-top: 16px; text-align: center"
            v-if="fileList.length > 0"
          >
            <a-button
              type="primary"
              :loading="loading"
              @click="handleUploadScan"
              >Start Scan</a-button
            >
          </div>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<script lang="ts">
import { defineComponent, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { message } from "ant-design-vue";
import { InboxOutlined } from "@ant-design/icons-vue";
import type { UploadProps } from "ant-design-vue";
import request from "@/utils/request";

export default defineComponent({
  name: "CodeqlScan",
  components: {
    InboxOutlined,
  },
  setup() {
    const router = useRouter();
    const activeTab = ref("git");
    const loading = ref(false);

    const ruleOptions = [
      { label: "CWE-078 (Command Injection)", value: "CWE-078" },
      { label: "CWE-502 (Deserialization)", value: "CWE-502" },
      { label: "CWE-094 (Code Injection)", value: "CWE-094" },
      { label: "CWE-918 (SSRF)", value: "CWE-918" },
      { label: "CWE-611 (XXE)", value: "CWE-611" },
      { label: "CWE-089 (SQL Injection)", value: "CWE-089" },
      { label: "CWE-079 (XSS)", value: "CWE-079" },
    ];

    const gitForm = reactive({
      repoUrl: "",
      branch: "main",
      rules: [] as string[],
    });

    const fileList = ref<UploadProps["fileList"]>([]);

    const handleGitScan = async () => {
      if (!gitForm.repoUrl) {
        message.error("Please input repository URL");
        return;
      }
      if (gitForm.rules.length === 0) {
        message.error("Please select at least one rule");
        return;
      }

      loading.value = true;
      try {
        await request.post("/api/sast/codeql/create", gitForm);
        message.success("Scan task started successfully");
        router.push("/sast/vuln");
      } catch (error) {
        message.error("Failed to start scan task");
        console.error(error);
      } finally {
        loading.value = false;
      }
    };

    const beforeUpload: UploadProps["beforeUpload"] = (file) => {
      fileList.value = [file];
      return false; // Prevent automatic upload
    };

    const handleRemove: UploadProps["onRemove"] = () => {
      fileList.value = [];
    };

    const handleUploadScan = async () => {
      if (fileList.value && fileList.value.length === 0) {
        message.error("Please upload a file first");
        return;
      }

      loading.value = true;
      const formData = new FormData();
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const file = fileList.value![0] as any;
      formData.append("file", file.originFileObj || file);

      try {
        await request.post("/api/sast/codeql/upload", formData, {
          headers: {
            "Content-Type": "multipart/form-data",
          },
        });
        message.success("Database uploaded and scan started");
        router.push("/sast/vuln");
      } catch (error) {
        message.error("Failed to upload and start scan");
        console.error(error);
      } finally {
        loading.value = false;
      }
    };

    return {
      activeTab,
      loading,
      gitForm,
      ruleOptions,
      fileList,
      handleGitScan,
      beforeUpload,
      handleRemove,
      handleUploadScan,
    };
  },
});
</script>

<style scoped>
.codeql-scan-container {
  padding: 24px;
}
</style>
