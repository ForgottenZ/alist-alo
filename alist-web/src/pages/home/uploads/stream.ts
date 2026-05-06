import { getSettingNumber, password } from "~/store"
import { EmptyResp, Resp } from "~/types"
import { r } from "~/utils"
import { SetUpload, Upload } from "./types"
import { calculateHash } from "./util"

const calcSpeed = (
  loaded: number,
  oldLoaded: number,
  oldTimestamp: number,
  setUpload: SetUpload,
) => {
  const timestamp = new Date().valueOf()
  const duration = (timestamp - oldTimestamp) / 1000
  if (duration > 1) {
    setUpload("speed", (loaded - oldLoaded) / duration)
    return { timestamp, loaded }
  }
  return { timestamp: oldTimestamp, loaded: oldLoaded }
}

export const StreamUpload: Upload = async (
  uploadPath: string,
  file: File,
  setUpload: SetUpload,
  asTask = false,
  overwrite = false,
  rapid = false,
  chunked = false,
  chunkSize = getSettingNumber("web_chunk_upload_part_size", 10485760),
  tempInTargetDir = false,
): Promise<Error | undefined> => {
  let oldTimestamp = new Date().valueOf()
  let oldLoaded = 0
  const headers: { [k: string]: any } = {
    "File-Path": encodeURIComponent(uploadPath),
    "As-Task": asTask,
    "Content-Type": file.type || "application/octet-stream",
    "Last-Modified": file.lastModified,
    Password: password(),
    Overwrite: overwrite.toString(),
  }
  if (rapid) {
    const { md5, sha1, sha256 } = await calculateHash(file)
    headers["X-File-Md5"] = md5
    headers["X-File-Sha1"] = sha1
    headers["X-File-Sha256"] = sha256
  }

  const enableChunked = chunked && file.size > chunkSize && chunkSize > 0
  if (enableChunked) {
    const totalChunks = Math.ceil(file.size / chunkSize)
    setUpload("totalChunks", totalChunks)
    const uploadId = encodeURIComponent(
      `${uploadPath}-${file.size}-${file.lastModified}`,
    )
    const chunkHeaders = {
      ...headers,
      "Upload-Id": uploadId,
      "Chunk-Size": chunkSize,
      "Resume-Upload": "true",
      "Chunk-Temp-In-Target": tempInTargetDir.toString(),
    }
    type ChunkStatus = {
      last_chunk: number
      next_chunk: number
      total_chunks: number
      total_size: number
    }
    const statusResp: Resp<ChunkStatus> = await r.get("/fs/chunk/status", {
      headers: chunkHeaders,
    })
    let startChunk = 0
    if (statusResp.code !== 200) {
      return new Error(statusResp.message)
    }
    if (statusResp.data) {
      startChunk = Math.max(
        0,
        Math.min(statusResp.data.next_chunk, totalChunks - 1),
      )
      if (startChunk > 0) {
        const loaded = Math.min(startChunk * chunkSize, file.size)
        setUpload("progress", ((loaded / file.size) * 100) | 0)
        oldLoaded = loaded
      }
    }
    for (let chunkIndex = startChunk; chunkIndex < totalChunks; chunkIndex++) {
      setUpload("currentChunk", chunkIndex + 1)
      const start = chunkIndex * chunkSize
      const end = Math.min(start + chunkSize, file.size)
      const chunk = file.slice(start, end)
      const resp: EmptyResp = await r.put("/fs/put", chunk, {
        headers: {
          ...chunkHeaders,
          "Chunk-Index": chunkIndex,
          "Total-Chunks": totalChunks,
          "Total-Size": file.size,
        },
        onUploadProgress: (progressEvent) => {
          const chunkLoaded = progressEvent.loaded || 0
          const loaded = start + chunkLoaded
          setUpload("progress", ((loaded / file.size) * 100) | 0)
          const speedData = calcSpeed(loaded, oldLoaded, oldTimestamp, setUpload)
          oldTimestamp = speedData.timestamp
          oldLoaded = speedData.loaded
        },
      })
      if (resp.code !== 200) {
        return new Error(resp.message)
      }
    }
    setUpload("status", "backending")
    return
  }

  setUpload("currentChunk", undefined)
  setUpload("totalChunks", undefined)

  const resp: EmptyResp = await r.put("/fs/put", file, {
    headers,
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        const complete = ((progressEvent.loaded / progressEvent.total) * 100) | 0
        setUpload("progress", complete)
        const speedData = calcSpeed(
          progressEvent.loaded,
          oldLoaded,
          oldTimestamp,
          setUpload,
        )
        oldTimestamp = speedData.timestamp
        oldLoaded = speedData.loaded
        if (complete === 100) {
          setUpload("status", "backending")
        }
      }
    },
  })
  if (resp.code !== 200) {
    return new Error(resp.message)
  }
}
