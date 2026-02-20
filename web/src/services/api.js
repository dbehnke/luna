const API_BASE = ''

export function getPersonaDisplayName(item) {
  if (item.persona && typeof item.persona === 'object') {
    return item.persona.display_name || null
  }
  if (item.persona_display_name) {
    return item.persona_display_name
  }
  if (item.persona_name) {
    return item.persona_name
  }
  return null
}

export function getPersonaAvatarUrl(item) {
  if (item.persona && typeof item.persona === 'object') {
    return item.persona.avatar_url || null
  }
  if (item.persona_avatar_url) {
    return item.persona_avatar_url
  }
  return null
}

export function getPersonaSlug(item) {
  if (item.persona && typeof item.persona === 'object') {
    return item.persona.slug || null
  }
  if (item.persona_slug) {
    return item.persona_slug
  }
  return null
}

export async function createItem(type, title = '', description = '', personaId = null) {
  const res = await fetch(`${API_BASE}/api/items`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type,
      title,
      description,
      persona_id: personaId
    })
  })
  if (!res.ok) throw new Error('Failed to create item')
  return res.json()
}

export async function uploadFile(itemId, file) {
  const formData = new FormData()
  formData.append('file', file)
  
  const res = await fetch(`${API_BASE}/api/items/${itemId}/upload`, {
    method: 'POST',
    body: formData
  })
  if (!res.ok) throw new Error('Failed to upload file')
  return res.json()
}

export async function listItems(options = {}) {
  const params = new URLSearchParams()
  if (options.type) params.append('type', options.type)
  if (options.user === 'me') params.append('user', 'me')
  if (options.limit) params.append('limit', options.limit)
  if (options.cursor) params.append('cursor', options.cursor)
  
  const res = await fetch(`${API_BASE}/api/items?${params}`)
  if (!res.ok) throw new Error('Failed to list items')
  return res.json()
}

export async function getItem(id) {
  const res = await fetch(`${API_BASE}/api/items/${id}`)
  if (!res.ok) throw new Error('Failed to get item')
  return res.json()
}

export async function deleteItem(id) {
  const res = await fetch(`${API_BASE}/api/items/${id}`, {
    method: 'DELETE'
  })
  if (!res.ok) throw new Error('Failed to delete item')
  return res.json()
}

export async function createClip(itemId, startMs, endMs) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/clip`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start_ms: startMs, end_ms: endMs })
  })
  if (!res.ok) throw new Error('Failed to create clip')
  return res.json()
}

export async function getItemClips(itemId) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/clips`)
  if (!res.ok) throw new Error('Failed to get clips')
  return res.json()
}

export async function listShorts(options = {}) {
  const params = new URLSearchParams()
  if (options.limit) params.append('limit', options.limit)
  if (options.cursor) params.append('cursor', options.cursor)
  
  const res = await fetch(`${API_BASE}/api/shorts?${params}`)
  if (!res.ok) throw new Error('Failed to list shorts')
  return res.json()
}

export async function setReaction(itemId, value) {
  const res = await fetch(`${API_BASE}/api/items/${itemId}/reaction`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value })
  })
  if (!res.ok) throw new Error('Failed to set reaction')
  return res.json()
}

export async function getPersonas() {
  const res = await fetch(`${API_BASE}/api/personas`)
  if (!res.ok) throw new Error('Failed to get personas')
  return res.json()
}

export async function createPersona(displayName) {
  const res = await fetch(`${API_BASE}/api/personas`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ display_name: displayName })
  })
  if (!res.ok) throw new Error('Failed to create persona')
  return res.json()
}

export async function uploadPersonaAvatar(personaId, file) {
  const formData = new FormData()
  formData.append('file', file)
  
  const res = await fetch(`${API_BASE}/api/personas/${personaId}/avatar`, {
    method: 'POST',
    body: formData
  })
  if (!res.ok) throw new Error('Failed to upload avatar')
  return res.json()
}

export async function getProfile(slug) {
  const res = await fetch(`${API_BASE}/api/profile/${encodeURIComponent(slug)}`)
  if (!res.ok) throw new Error('Failed to get profile')
  return res.json()
}

export async function getProfileItems(slug, options = {}) {
  const params = new URLSearchParams()
  if (options.type) params.append('type', options.type)
  if (options.limit) params.append('limit', options.limit)
  if (options.cursor) params.append('cursor', options.cursor)
  
  const res = await fetch(`${API_BASE}/api/profile/${encodeURIComponent(slug)}/items?${params}`)
  if (!res.ok) throw new Error('Failed to get profile items')
  return res.json()
}

export async function getProfileShorts(slug, options = {}) {
  const params = new URLSearchParams()
  if (options.limit) params.append('limit', options.limit)
  if (options.cursor) params.append('cursor', options.cursor)
  
  const res = await fetch(`${API_BASE}/api/profile/${encodeURIComponent(slug)}/shorts?${params}`)
  if (!res.ok) throw new Error('Failed to get profile shorts')
  return res.json()
}
