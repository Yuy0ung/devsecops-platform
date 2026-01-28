<template>
  <div class="sca-list-container">
    <a-card title="SCA Project List" :bordered="false" style="width: 100%">
      <template #extra>
        <a-space>
          <a-button type="primary" @click="showCreateModal">New Scan</a-button>
          <a-popconfirm
            v-if="hasSelected"
            title="Are you sure delete?"
            ok-text="Yes"
            cancel-text="No"
            @confirm="handleBulkDelete"
          >
            <a-button type="primary" danger :loading="loading">
              Delete
            </a-button>
          </a-popconfirm>
          <a-button @click="fetchTasks">Refresh</a-button>
        </a-space>
      </template>

      <div style="margin-bottom: 16px" v-if="hasSelected">
        <span style="margin-left: 8px">
          Selected {{ selectedRowKeys.length }} items
        </span>
      </div>

      <a-table
        :columns="columns"
        :data-source="tasks"
        :loading="loading"
        row-key="id"
        :row-selection="{
          selectedRowKeys: selectedRowKeys,
          onChange: onSelectChange,
        }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="getStatusColor(record.status)">
              {{ record.status.toUpperCase() }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'projectLanguage'">
            <a-tag
              v-for="lang in record.projectLanguage
                ? record.projectLanguage.split(', ')
                : []"
              :key="lang"
              color="blue"
            >
              {{ lang }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="viewDetails(record.id)"
              >View Results</a-button
            >
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:visible="createVisible"
      title="Create SCA Scan Task"
      @ok="handleCreate"
      :confirmLoading="createLoading"
    >
      <a-form layout="vertical">
        <a-form-item label="Upload File (Jar/War/Zip)" required>
          <a-upload
            v-model:file-list="fileList"
            :before-upload="beforeUpload"
            :max-count="1"
            accept=".jar,.war,.zip"
          >
            <a-button>
              <upload-outlined />
              Select File
            </a-button>
          </a-upload>
          <a-progress
            v-if="createLoading && uploadProgress > 0"
            :percent="uploadProgress"
            size="small"
            status="active"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, onMounted, computed } from "vue";
import { useRouter } from "vue-router";
import request from "@/utils/request";
import { message } from "ant-design-vue";
import { UploadOutlined } from "@ant-design/icons-vue";

interface ScaTask {
  id: string;
  target: string;
  type: string;
  status: string;
  startTime: string;
  result: string;
  projectLanguage?: string;
}

export default defineComponent({
  name: "ScaList",
  components: {
    UploadOutlined,
  },
  setup() {
    const router = useRouter();
    const loading = ref(false);
    const tasks = ref<ScaTask[]>([]);
    const selectedRowKeys = ref<string[]>([]);
    const createVisible = ref(false);
    const createLoading = ref(false);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const fileList = ref<any[]>([]);

    const beforeUpload = (file: any) => {
      // Do not manually update fileList here, let AntDV handle it via v-model
      // Just return false to prevent auto upload
      return false;
    };

    const hasSelected = computed(() => selectedRowKeys.value.length > 0);

    const onSelectChange = (keys: string[]) => {
      selectedRowKeys.value = keys;
    };

    const columns = [
      {
        title: "Target",
        dataIndex: "target",
        key: "target",
        ellipsis: true,
      },
      {
        title: "Language",
        dataIndex: "projectLanguage",
        key: "projectLanguage",
        width: 120,
      },
      {
        title: "Status",
        dataIndex: "status",
        key: "status",
        width: 120,
      },
      {
        title: "Result",
        dataIndex: "result",
        key: "result",
        ellipsis: true,
      },
      {
        title: "Start Time",
        dataIndex: "startTime",
        key: "startTime",
        width: 200,
      },
      {
        title: "Action",
        key: "action",
        width: 150,
      },
    ];

    const getStatusColor = (status: string) => {
      switch (status) {
        case "completed":
          return "success";
        case "running":
          return "processing";
        case "failed":
          return "error";
        default:
          return "default";
      }
    };

    const fetchTasks = async () => {
      loading.value = true;
      try {
        const res = await request.get("/api/sca/list");
        tasks.value = res.data;
      } catch (error) {
        message.error("Failed to fetch tasks");
      } finally {
        loading.value = false;
      }
    };

    const viewDetails = (id: string) => {
      router.push(`/sca/vuln/${id}`);
    };

    const handleBulkDelete = async () => {
      loading.value = true;
      try {
        await Promise.all(
          selectedRowKeys.value.map((id) =>
            request.post(`/api/sca/vuln/delete/${id}`)
          )
        );
        message.success("Deleted successfully");
        selectedRowKeys.value = [];
        fetchTasks();
      } catch (error) {
        message.error("Failed to delete");
      } finally {
        loading.value = false;
      }
    };

    const showCreateModal = () => {
      fileList.value = [];
      createVisible.value = true;
    };

    const uploadProgress = ref(0);

    const handleCreate = async () => {
      if (fileList.value.length === 0) {
        message.error("Please select a file");
        return;
      }
      createLoading.value = true;
      uploadProgress.value = 0;

      try {
        const file = fileList.value[0];
        // Check if it's a wrapper object (Ant Design Vue) or the raw file
        const targetFile = file.originFileObj || file;

        // Use chunked upload for files > 10MB
        if (targetFile.size > 10 * 1024 * 1024) {
          await handleChunkedUpload(targetFile);
        } else {
          const formData = new FormData();
          formData.append("file", targetFile);

          await request.post("/api/sca/create", formData, {
            timeout: 600000,
          });
          message.success("Task created");
          createVisible.value = false;
          fileList.value = [];
          fetchTasks();
        }
      } catch (error) {
        console.error(error);
        message.error("Failed to create task");
      } finally {
        createLoading.value = false;
        uploadProgress.value = 0;
      }
    };

    const handleChunkedUpload = async (file: File) => {
      const chunkSize = 5 * 1024 * 1024; // 5MB per chunk
      const totalChunks = Math.ceil(file.size / chunkSize);

      // 1. Init upload
      const initRes = await request.post("/api/sca/upload/init");
      const uploadId = initRes.data.uploadId;

      // 2. Upload chunks (Parallel with limit)
      const concurrency = 3; // Upload 3 chunks at a time
      let completed = 0;

      const uploadChunk = async (index: number) => {
        const start = index * chunkSize;
        const end = Math.min(start + chunkSize, file.size);
        const chunk = file.slice(start, end);

        const formData = new FormData();
        formData.append("uploadId", uploadId);
        formData.append("chunkIndex", index.toString());
        formData.append("file", chunk);

        await request.post("/api/sca/upload/chunk", formData, {
          timeout: 600000, // 10 min timeout per chunk
        });

        completed++;
        uploadProgress.value = Math.floor((completed / totalChunks) * 100);
      };

      // Queue management
      const queue = Array.from({ length: totalChunks }, (_, i) => i);
      const workers = Array(concurrency)
        .fill(null)
        .map(async () => {
          while (queue.length > 0) {
            const index = queue.shift();
            if (index !== undefined) {
              await uploadChunk(index);
            }
          }
        });

      await Promise.all(workers);

      // 3. Merge
      const formData = new FormData();
      formData.append("uploadId", uploadId);
      formData.append("fileName", file.name);
      formData.append("totalChunks", totalChunks.toString());

      await request.post("/api/sca/upload/merge", formData);

      message.success("Task created");
      createVisible.value = false;
      fileList.value = [];
      fetchTasks();
    };

    onMounted(() => {
      fetchTasks();
    });

    return {
      tasks,
      loading,
      columns,
      selectedRowKeys,
      hasSelected,
      onSelectChange,
      getStatusColor,
      fetchTasks,
      viewDetails,
      handleBulkDelete,
      createVisible,
      createLoading,
      showCreateModal,
      handleCreate,
      fileList,
      beforeUpload,
      uploadProgress,
    };
  },
});
</script>
