export interface AutomationOperation {
  type: string
  source: string
  destination?: string
  new_name?: string
  inner_path?: string
  password?: string
  cache_full?: boolean
  put_into_new_dir?: boolean
}

export interface AutomationTask {
  id: number
  name: string
  description: string
  enabled: boolean
  schedule_type: string
  interval_value: number
  interval_unit: string
  time_of_day: string
  weekdays: number[]
  once_at?: string | null
  next_run?: string | null
  last_run?: string | null
  last_status?: string | null
  last_error?: string | null
  operations: AutomationOperation[]
}

export interface AutomationHistory {
  id: number
  executed_at: string
  status: string
  message: string
  duration: number
}
