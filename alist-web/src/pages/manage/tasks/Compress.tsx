import { useManageTitle } from "~/hooks"
import { TypeTasks } from "~/pages/manage/tasks/Tasks"
import { getCompressNameAnalyzer } from "~/pages/manage/tasks/helper"

const Compress = () => {
  useManageTitle("manage.sidemenu.compress")
  return (
    <TypeTasks type="compress" canRetry nameAnalyzer={getCompressNameAnalyzer()} />
  )
}

export default Compress
