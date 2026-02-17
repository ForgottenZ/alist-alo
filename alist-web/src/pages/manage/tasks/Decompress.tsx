import { useManageTitle } from "~/hooks"
import { VStack } from "@hope-ui/solid"
import { TypeTasks } from "~/pages/manage/tasks/Tasks"
import {
  getCompressNameAnalyzer,
  getDecompressNameAnalyzer,
  getDecompressUploadNameAnalyzer,
} from "~/pages/manage/tasks/helper"

const Decompress = () => {
  useManageTitle("manage.sidemenu.decompress")
  return (
    <VStack w="$full" alignItems="start" spacing="$4">
      <TypeTasks
        type="decompress"
        canRetry
        nameAnalyzer={getDecompressNameAnalyzer()}
      />
      <TypeTasks
        type="decompress_upload"
        canRetry
        nameAnalyzer={getDecompressUploadNameAnalyzer()}
      />
      <TypeTasks type="compress" canRetry nameAnalyzer={getCompressNameAnalyzer()} />
    </VStack>
  )
}

export default Decompress
