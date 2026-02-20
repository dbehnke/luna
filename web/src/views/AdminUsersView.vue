<template>
  <div class="min-h-screen bg-[#07182c] p-4">
    <header class="flex items-center justify-between mb-6">
      <router-link to="/" class="text-gray-400 hover:text-white">
        ← Back
      </router-link>
      <h1 class="text-xl font-bold text-white">Admin · Users</h1>
      <button
        @click="refreshUsers"
        class="bg-gray-700 hover:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
      >
        Refresh
      </button>
    </header>

    <main class="space-y-6">
      <section class="bg-[#0a2540] rounded-lg p-4">
        <h2 class="text-white font-semibold mb-3">Create User</h2>
        <form class="grid grid-cols-1 md:grid-cols-4 gap-3" @submit.prevent="createUser">
          <input
            v-model="createForm.username"
            type="text"
            placeholder="username"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            required
          />
          <input
            v-model="createForm.password"
            type="password"
            placeholder="password"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            required
          />
          <select
            v-model="createForm.role"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="user">user</option>
            <option value="admin">admin</option>
          </select>
          <button
            type="submit"
            :disabled="busy"
            class="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white px-4 py-2 rounded-lg transition-colors"
          >
            Create
          </button>
        </form>
      </section>

      <section class="bg-[#0a2540] rounded-lg p-4">
        <div class="flex items-center justify-between mb-3">
          <h2 class="text-white font-semibold">Users</h2>
          <select
            v-model="statusFilter"
            class="bg-gray-800 text-white px-3 py-2 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            @change="refreshUsers"
          >
            <option value="all">all</option>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
          </select>
        </div>

        <p v-if="error" class="text-red-400 mb-3">{{ error }}</p>
        <p v-if="message" class="text-green-400 mb-3">{{ message }}</p>

        <div v-if="loading" class="text-gray-400">Loading users...</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm text-gray-200">
            <thead>
              <tr class="text-gray-400 border-b border-gray-700">
                <th class="py-2">Username</th>
                <th class="py-2">Role</th>
                <th class="py-2">Status</th>
                <th class="py-2">Created</th>
                <th class="py-2">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="user in users"
                :key="user.id"
                class="border-b border-gray-800"
              >
                <td class="py-2">{{ user.username }}</td>
                <td class="py-2">
                  <select
                    :value="user.role"
                    class="bg-gray-800 text-white px-2 py-1 rounded"
                    :disabled="busy"
                    @change="changeRole(user, $event.target.value)"
                  >
                    <option value="user">user</option>
                    <option value="admin">admin</option>
                  </select>
                </td>
                <td class="py-2">
                  <span
                    :class="user.is_active ? 'text-green-400' : 'text-amber-400'"
                  >
                    {{ user.is_active ? "active" : "inactive" }}
                  </span>
                </td>
                <td class="py-2">{{ formatDate(user.created_at) }}</td>
                <td class="py-2">
                  <div class="flex flex-wrap gap-2">
                    <button
                      v-if="user.is_active"
                      :disabled="busy"
                      class="bg-amber-700 hover:bg-amber-600 disabled:bg-gray-600 text-white px-2 py-1 rounded"
                      @click="deactivate(user)"
                    >
                      Deactivate
                    </button>
                    <button
                      v-else
                      :disabled="busy"
                      class="bg-green-700 hover:bg-green-600 disabled:bg-gray-600 text-white px-2 py-1 rounded"
                      @click="activate(user)"
                    >
                      Activate
                    </button>
                    <button
                      :disabled="busy"
                      class="bg-blue-700 hover:bg-blue-600 disabled:bg-gray-600 text-white px-2 py-1 rounded"
                      @click="resetPassword(user)"
                    >
                      Reset Password
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup>
import { onMounted, ref } from "vue";
import {
  adminActivateUser,
  adminCreateUser,
  adminDeactivateUser,
  adminListUsers,
  adminSetUserPassword,
  adminSetUserRole,
} from "../services/api";

const users = ref([]);
const loading = ref(true);
const busy = ref(false);
const error = ref("");
const message = ref("");
const statusFilter = ref("all");
const createForm = ref({
  username: "",
  password: "",
  role: "user",
});

function formatDate(value) {
  return new Date(value).toLocaleString();
}

function setFeedback(errText = "", msgText = "") {
  error.value = errText;
  message.value = msgText;
}

async function refreshUsers() {
  loading.value = true;
  setFeedback();
  try {
    const result = await adminListUsers(statusFilter.value);
    users.value = result.users || [];
  } catch (e) {
    setFeedback(e.message || "Failed to load users");
  } finally {
    loading.value = false;
  }
}

async function createUser() {
  busy.value = true;
  setFeedback();
  try {
    await adminCreateUser(
      createForm.value.username,
      createForm.value.password,
      createForm.value.role,
    );
    setFeedback("", "User created");
    createForm.value.username = "";
    createForm.value.password = "";
    createForm.value.role = "user";
    await refreshUsers();
  } catch (e) {
    setFeedback(e.message || "Failed to create user");
  } finally {
    busy.value = false;
  }
}

async function changeRole(user, role) {
  busy.value = true;
  setFeedback();
  try {
    await adminSetUserRole(user.username, role);
    setFeedback("", `Role updated for ${user.username}`);
    await refreshUsers();
  } catch (e) {
    setFeedback(e.message || "Failed to update role");
  } finally {
    busy.value = false;
  }
}

async function deactivate(user) {
  busy.value = true;
  setFeedback();
  try {
    await adminDeactivateUser(user.username);
    setFeedback("", `Deactivated ${user.username}`);
    await refreshUsers();
  } catch (e) {
    setFeedback(e.message || "Failed to deactivate user");
  } finally {
    busy.value = false;
  }
}

async function activate(user) {
  busy.value = true;
  setFeedback();
  try {
    await adminActivateUser(user.username);
    setFeedback("", `Activated ${user.username}`);
    await refreshUsers();
  } catch (e) {
    setFeedback(e.message || "Failed to activate user");
  } finally {
    busy.value = false;
  }
}

async function resetPassword(user) {
  const nextPassword = window.prompt(`Set new password for ${user.username}`);
  if (!nextPassword) return;

  busy.value = true;
  setFeedback();
  try {
    await adminSetUserPassword(user.username, nextPassword);
    setFeedback("", `Password reset for ${user.username}`);
  } catch (e) {
    setFeedback(e.message || "Failed to reset password");
  } finally {
    busy.value = false;
  }
}

onMounted(() => {
  refreshUsers();
});
</script>
