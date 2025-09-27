export interface AutomationStepOption {
  [key: string]: string
}

export interface AutomationStep {
  id?: number
  sort_order: number
  action: string
  source: string
  target?: string
  options?: AutomationStepOption
}

export interface AutomationTask {
  id: number
  name: string
  enabled: boolean
  schedule_type: string
  interval_value: number
  interval_unit: string
  weekdays: number[]
  time_of_day: string
  specific_time: string
  last_run_at?: string | null
  next_run_at?: string | null
  status: string
  last_message: string
  steps: AutomationStep[]
}

export interface AutomationHistoryItem {
  id: number
  status: string
  message: string
  started_at: string
  finished_at: string
}

export interface AutomationTaskPayload {
  id?: number
  name: string
  enabled: boolean
  schedule_type: string
  interval_value: number
  interval_unit: string
  weekdays: number[]
  time_of_day: string
  specific_time: string
  steps: AutomationStep[]
}
