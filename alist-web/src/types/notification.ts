export type NotificationType = "pushdeer" | "azure_oauth"

export interface NotificationItem {
  id: number
  name: string
  type: NotificationType
  enabled: boolean
  config: string
  remark: string
  created_at?: string
  updated_at?: string
}
