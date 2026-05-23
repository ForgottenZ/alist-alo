export interface TaskInfo {
  id: string
  name: string
  creator: string
  creator_role: number
  state: number
  status: string
  progress: number
  start_time: string | null
  end_time: string | null
  total_bytes: number
  error: string
  has_notification: boolean
  notification_id?: number
  notification_name?: string
  notification_event_id?: string
}
