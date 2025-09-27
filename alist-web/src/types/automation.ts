export interface AutomationSchedule {
  mode: "interval" | "weekly" | "once"
  interval_value?: number
  interval_unit?: "second" | "minute" | "hour" | "day"
  weekly_day?: string
  time_of_day?: string
  once_at?: string
}

export interface AutomationStepOptions {
  new_name?: string
  inner_path?: string
  password?: string
  put_into_new_dir?: boolean
  cache_full?: boolean
}

export interface AutomationStep {
  action: string
  source: string
  target?: string
  description?: string
  options: AutomationStepOptions
}

export interface AutomationHistory {
  id: number
  success: boolean
  message: string
  started_at: string
  finished_at?: string | null
}

export interface AutomationJob {
  id: number
  name: string
  enabled: boolean
  schedule: AutomationSchedule
  next_run_at?: string | null
  last_run_at?: string | null
  creator_id: number
  steps: AutomationStep[]
  histories: AutomationHistory[]
}

export interface AutomationJobPayload {
  id?: number
  name: string
  enabled: boolean
  schedule: AutomationSchedule
  steps: AutomationStep[]
}
