import { useManageTitle } from "~/hooks"
import { TypeTasks } from "./Tasks"
import { getCompressNameAnalyzer } from "./helper"

const Compress = () => {
  useManageTitle("manage.sidemenu.compress")
  return (
    <TypeTasks type="compress" canRetry nameAnalyzer={getCompressNameAnalyzer()} />
  )
}

export default Compress
