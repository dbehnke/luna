const API_BASE = "";

export function getPersonaDisplayName(item) {
  if (item.persona && typeof item.persona === "object") {
    return item.persona.display_name || null;
  }
  if (item.persona_display_name) {
    return item.persona_display_name;
  }
  if (item.persona_name) {
    return item.persona_name;
  }
  return null;
}

export function getPersonaAvatarUrl(item) {
  if (item.persona && typeof item.persona === "object") {
    return item.persona.avatar_url || null;
  }
  if (item.persona_avatar_url) {
    return item.persona_avatar_url;
  }
  return null;
}

export function getPersonaSlug(item) {
  if (item.persona && typeof item.persona === "object") {
    return item.persona.slug || null;
  }
  if (item.persona_slug) {
    return item.persona_slug;
  }
  return null;
}

export async function createItem(
  type,
  title = "",
  description = "",
  personaId = null,
) {
  const res = await fetch(`${API_BASE}/api/items`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      type,
      title,
      description,
      persona_id: personaId,
    }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || "Failed to create item");
  }
  return res.json();
}

export async function uploadFile(itemId, file) {
  const formData = new FormData();
  formData.append("file", file);

  const res = await fetch(`${API_BASE}/api/items/${itemId}/upload`, {
    method: "POST",
    body: formData,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || "Failed to upload file");
  }
  return res.json();
}

export async function listItems(options = {}) {
  const params = new URLSearchParams();
  if (options.q) params.append("q", options.q);
  if (options.type) params.append("type", options.type);
  if (options.persona_id) params.append("persona_id", options.persona_id);
  if (options.user === "me") params.append("user", "me");
  if (options.favorites) params.append("favorites", "1");
  if (options.highlighted) params.append("highlighted", "1");
  if (options.sort) params.append("sort", options.sort);
  if (options.limit) params.append("limit", options.limit);
  if (options.cursor) params.append("cursor", options.cursor);

  const res = await fetch(`${API_BASE}/api/items?${params}`);
  if (!res.ok) throw new Error("Failed to list items");
  return res.json();
}

export async function getItem(id) {
  const res = await fetch(`${API_BASE}/api/items/${id}`);
  if (!res.ok) throw new Error("Failed to get item");
  return res.json();
}

export async function deleteItem(id) {
  const res = await fetch(`${API_BASE}/api/items/${id}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error("Failed to delete item");
  return res.json();
}

export async function createClip(itemId, startMs, endMs) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/clip`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ start_ms: startMs, end_ms: endMs }),
  });
  if (!res.ok) throw new Error("Failed to create clip");
  return res.json();
}

export async function getItemClips(itemId) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/clips`);
  if (!res.ok) throw new Error("Failed to get clips");
  return res.json();
}

export async function listShorts(options = {}) {
  const params = new URLSearchParams();
  if (options.limit) params.append("limit", options.limit);
  if (options.cursor) params.append("cursor", options.cursor);

  const res = await fetch(`${API_BASE}/api/shorts?${params}`);
  if (!res.ok) throw new Error("Failed to list shorts");
  return res.json();
}

export async function setReaction(itemId, value) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/reaction`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value }),
  });
  if (!res.ok) throw new Error("Failed to set reaction");
  return res.json();
}

export async function setFavorite(itemId, enabled) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/favorite`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  if (!res.ok) throw new Error("Failed to set favorite");
  return res.json();
}

export async function setHighlight(itemId, enabled) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/highlight`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  if (!res.ok) throw new Error("Failed to set highlight");
  return res.json();
}

export async function getPersonas() {
  const res = await fetch(`${API_BASE}/api/personas`);
  if (!res.ok) throw new Error("Failed to get personas");
  return res.json();
}

export async function createPersona(displayName) {
  const res = await fetch(`${API_BASE}/api/personas`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ display_name: displayName }),
  });
  if (!res.ok) throw new Error("Failed to create persona");
  return res.json();
}

export async function uploadPersonaAvatar(personaId, file) {
  const formData = new FormData();
  formData.append("file", file);

  const res = await fetch(`${API_BASE}/api/personas/${personaId}/avatar`, {
    method: "POST",
    body: formData,
  });
  if (!res.ok) throw new Error("Failed to upload avatar");
  return res.json();
}

export async function getProfile(slug) {
  const res = await fetch(
    `${API_BASE}/api/profile/${encodeURIComponent(slug)}`,
  );
  if (!res.ok) throw new Error("Failed to get profile");
  return res.json();
}

export async function getProfileItems(slug, options = {}) {
  const params = new URLSearchParams();
  if (options.type) params.append("type", options.type);
  if (options.q) params.append("q", options.q);
  if (options.highlighted) params.append("highlighted", "1");
  if (options.sort) params.append("sort", options.sort);
  if (options.limit) params.append("limit", options.limit);
  if (options.cursor) params.append("cursor", options.cursor);

  const res = await fetch(
    `${API_BASE}/api/profile/${encodeURIComponent(slug)}/items?${params}`,
  );
  if (!res.ok) throw new Error("Failed to get profile items");
  return res.json();
}

export async function getProfileShorts(slug, options = {}) {
  const params = new URLSearchParams();
  if (options.limit) params.append("limit", options.limit);
  if (options.cursor) params.append("cursor", options.cursor);

  const res = await fetch(
    `${API_BASE}/api/profile/${encodeURIComponent(slug)}/shorts?${params}`,
  );
  if (!res.ok) throw new Error("Failed to get profile shorts");
  return res.json();
}

export async function getCurrentUser() {
  const res = await fetch(`${API_BASE}/api/me`);
  if (!res.ok) return null;
  return res.json();
}

export async function login(username, password) {
  const res = await fetch(`${API_BASE}/api/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || "Login failed");
  }
  return res.json();
}

export async function logout() {
  const res = await fetch(`${API_BASE}/api/logout`, {
    method: "POST",
  });
  if (!res.ok) throw new Error("Logout failed");
  return res.json();
}

export async function adminListUsers(status = "all") {
  const params = new URLSearchParams();
  if (status) params.append("status", status);

  const res = await fetch(`${API_BASE}/api/admin/users?${params.toString()}`);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function adminCreateUser(username, password, role = "user") {
  const res = await fetch(`${API_BASE}/api/admin/users`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password, role }),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function adminSetUserRole(username, role) {
  const res = await fetch(
    `${API_BASE}/api/admin/users/${encodeURIComponent(username)}/role`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ role }),
    },
  );
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function adminDeactivateUser(username) {
  const res = await fetch(
    `${API_BASE}/api/admin/users/${encodeURIComponent(username)}/deactivate`,
    {
      method: "POST",
    },
  );
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function adminActivateUser(username) {
  const res = await fetch(
    `${API_BASE}/api/admin/users/${encodeURIComponent(username)}/activate`,
    {
      method: "POST",
    },
  );
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function adminSetUserPassword(username, password) {
  const res = await fetch(
    `${API_BASE}/api/admin/users/${encodeURIComponent(username)}/password`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    },
  );
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function listPlaylists(includeItems = true) {
  const params = new URLSearchParams();
  if (includeItems) params.append("include_items", "1");
  const res = await fetch(`${API_BASE}/api/playlists?${params.toString()}`);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function createPlaylist(name) {
  const res = await fetch(`${API_BASE}/api/playlists`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function deletePlaylist(id) {
  const res = await fetch(`${API_BASE}/api/playlists/${id}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function addPlaylistItem(playlistId, itemId, position) {
  const body = { item_id: itemId };
  if (typeof position === "number") body.position = position;
  const res = await fetch(`${API_BASE}/api/playlists/${playlistId}/items`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function removePlaylistItem(playlistId, itemId) {
  const res = await fetch(
    `${API_BASE}/api/playlists/${playlistId}/items/${encodeURIComponent(itemId)}`,
    {
      method: "DELETE",
    },
  );
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function reorderPlaylist(playlistId, itemIds) {
  const res = await fetch(`${API_BASE}/api/playlists/${playlistId}/reorder`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ item_ids: itemIds }),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function listTrashItems(scope = "mine") {
  const params = new URLSearchParams();
  if (scope === "all") params.append("scope", "all");
  const res = await fetch(`${API_BASE}/api/items/trash?${params.toString()}`);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function restoreItem(id) {
  const res = await fetch(`${API_BASE}/api/items/${id}/restore`, {
    method: "POST",
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function purgeItem(id) {
  const res = await fetch(`${API_BASE}/api/items/${id}/purge`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
