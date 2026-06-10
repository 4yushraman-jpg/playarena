// Backend webhooks.Response — no created_by field
export interface WebhookEndpoint {
  id: string
  organization_id: string
  url: string
  description: string | null
  active: boolean
  created_at: string
  updated_at: string
}

// Backend webhooks.CreateResponse — raw secret exposed once, JSON key is "secret"
export interface WebhookCreateResponse extends WebhookEndpoint {
  secret: string
}

export interface WebhookListResponse {
  endpoints: WebhookEndpoint[]
  total: number
}

export interface CreateWebhookRequest {
  url: string
  description?: string
}

export interface UpdateWebhookActiveRequest {
  active: boolean
}
