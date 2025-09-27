import { useManageTitle } from "~/hooks"
import { TypeTasks } from "~/pages/manage/tasks/Tasks"

const Compress = () => {
  useManageTitle("manage.sidemenu.compress")
  return (
    <TypeTasks
      type="compress"
      canRetry
      nameAnalyzer={{
        regex: /^压缩 (.*) -> \[(.*)]\((.*)\)$/,
        title: (matches) => matches[1],
        attrs: {
          "目标挂载": (matches) => <span>{matches[2]}</span>,
          "目标路径": (matches) => <span>{matches[3]}</span>,
        },
      }}
    />
  )
}

export default Compress
