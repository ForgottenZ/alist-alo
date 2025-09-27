export interface AutomationOperation {
  id: string
  type: string
  source: string
  destination?: string
  new_name?: string
  password?: string
  inner_path?: string
  cache_full?: boolean
  put_into_new_dir?: boolean
}

export interface AutomationSchedule {
  mode: string
  once_at?: string
  interval_value?: number
  interval_unit?: string
  start_at?: string
  weekdays?: number[]
  time_of_day?: string
}

export interface AutomationOperationResult {
  operation: AutomationOperation
  targets: string[]
  success: boolean
  error?: string
}

export interface AutomationHistory {
  id: string
  started_at: string
  finished_at: string
  success: boolean
  message?: string
  results: AutomationOperationResult[]
}

export interface AutomationTask {
  id: string
  name: string
  enabled: boolean
  schedule: AutomationSchedule
  operations: AutomationOperation[]
  creator: string
  creator_role: number
  creator_id?: number
  created_at: string
  updated_at: string
  last_run?: string
  next_run?: string
  last_result?: string
  history: AutomationHistory[]
}

export interface AutomationListResp {
  tasks: AutomationTask[]
}

export interface AutomationTaskRequest {
  id?: string
  name: string
  enabled: boolean
  schedule: AutomationSchedule
  operations: AutomationOperation[]
}
