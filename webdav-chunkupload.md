# WebDAV 分块上传支持说明

本文说明如何在 AList 的 WebDAV `PUT` 接口中启用并使用分块上传。

## 1. 总体思路

- 继续沿用 `PUT /dav/...` 上传入口。
- 当请求头里携带分块信息时，服务端不直接落盘到目标存储，而是先把每个分块按顺序追加到临时文件。
- 当最后一个分块到达后，再将完整临时文件作为一次普通上传写入目标路径。

## 2. 请求头约定

客户端对每个分块请求都应附带以下头：

- `Chunk-Index`: 当前分块下标（从 `0` 开始）
- `Total-Chunks`: 总分块数（>0）
- `Total-Size`: 原始文件总大小（字节）
- `Upload-Id`: 本次上传的唯一 ID（同一个文件所有分块一致）
- `Content-Type`: 文件 MIME（可选，不传则按文件名推断）

## 3. 服务端处理流程

1. `handlePut` 检测到 `Chunk-Index` 后进入分块处理分支。
2. 校验 `Chunk-Index` / `Total-Chunks` / `Total-Size` / `Upload-Id` 的合法性。
3. 在 `TempDir/chunk_upload` 下，为 `Upload-Id + reqPath` 生成哈希临时文件名（避免冲突）。
4. 使用 `sync.Map + mutex` 对同一 `Upload-Id` 串行化写入，防止并发分块乱序追加。
5. 非最后分块：仅追加写入并返回 `200 OK`。
6. 最后分块：
   - 打开拼接后的临时文件。
   - 组装 `stream.FileStream`（文件名取请求路径 basename，大小用 `Total-Size`）。
   - 调用 `fs.PutDirectly(..., overwrite=true)` 写入实际存储。
   - 清理临时文件和锁。
   - 返回 `201 Created`。

## 4. 错误码建议

- 参数错误：`400 Bad Request`
- 目标目录不存在：`404 Not Found`
- 存储写入失败或其他错误：`405 Method Not Allowed`（保持现有 WebDAV `PUT` 风格）

## 5. 兼容性

- 不携带 `Chunk-Index` 时，仍走原有 WebDAV 普通上传逻辑。
- 因此对现有 WebDAV 客户端完全兼容；支持分块的客户端只需按约定追加请求头即可。
