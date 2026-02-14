import { password } from "~/store"
import { EmptyResp } from "~/types"
import { r } from "~/utils"
import { SetUpload, Upload } from "./types"
import { calculateHash } from "./util"

const createHeaders = (
  uploadPath: string,
  file: File,
  asTask: boolean,
  overwrite: boolean,
  hash?: Awaited<ReturnType<typeof calculateHash>>,
) => {
  const headers: { [k: string]: any } = {
    "File-Path": encodeURIComponent(uploadPath),
    "As-Task": asTask,
    "Content-Type": file.type || "application/octet-stream",
    "Last-Modified": file.lastModified,
    Password: password(),
    Overwrite: overwrite.toString(),
  }
  if (hash) {
    headers["X-File-Md5"] = hash.md5
    headers["X-File-Sha1"] = hash.sha1
    headers["X-File-Sha256"] = hash.sha256
  }
  return headers
}

export const StreamUpload: Upload = async (
  uploadPath: string,
  file: File,
  setUpload: SetUpload,
  asTask = false,
  overwrite = false,
  rapid = false,
  chunk = false,
  chunkSize = 0,
): Promise<Error | undefined> => {
  let oldTimestamp = new Date().valueOf()
  let oldLoaded = 0
  const hash = rapid ? await calculateHash(file) : undefined

  const useChunk = chunk && chunkSize > 0 && file.size > chunkSize
  if (!useChunk) {
    const headers = createHeaders(uploadPath, file, asTask, overwrite, hash)
    const resp: EmptyResp = await r.put("/fs/put", file, {
      headers,
      onUploadProgress: (progressEvent) => {
        if (!progressEvent.total) return
        const complete = ((progressEvent.loaded / progressEvent.total) * 100) | 0
        setUpload("progress", complete)
        const timestamp = new Date().valueOf()
        const duration = (timestamp - oldTimestamp) / 1000
        if (duration > 1) {
          const loaded = progressEvent.loaded - oldLoaded
          setUpload("speed", loaded / duration)
          oldTimestamp = timestamp
          oldLoaded = progressEvent.loaded
        }
        if (complete === 100) {
          setUpload("status", "backending")
        }
      },
    })
    return resp.code === 200 ? undefined : new Error(resp.message)
  }

  const total = Math.ceil(file.size / chunkSize)
  for (let i = 0; i < total; i++) {
    const start = i * chunkSize
    const end = Math.min(file.size, start + chunkSize)
    const blob = file.slice(start, end)
    const headers = createHeaders(uploadPath, file, false, overwrite, hash)
    headers["X-Chunk-Upload"] = "true"
    headers["X-Chunk-Index"] = `${i}`
    headers["X-Chunk-Total"] = `${total}`
    headers["X-File-Size"] = `${file.size}`
    const resp: EmptyResp = await r.put("/fs/put", blob, { headers })
    if (resp.code !== 200) {
      return new Error(resp.message)
    }
    const loaded = end
    const complete = ((loaded / file.size) * 100) | 0
    setUpload("progress", complete)
    const timestamp = new Date().valueOf()
    const duration = (timestamp - oldTimestamp) / 1000
    if (duration > 1) {
      const delta = loaded - oldLoaded
      setUpload("speed", delta / duration)
      oldTimestamp = timestamp
      oldLoaded = loaded
    }
    if (i === total - 1) {
      setUpload("status", "backending")
    }
  }
  return undefined
}
