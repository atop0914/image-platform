# image-platform

图片生成与审核一体化平台 (Go + Gin + MySQL)

## 功能特性

- 🤖 **多平台图片生成**: 支持 SiliconFlow、阿里云百炼 等多种模型提供商
- 📥 **自动入库**: 生成图片自动添加到审核队列
- ✅ **人工审核**: Web 界面审核通过/拒绝
- 📸 **当天图库**: 查看审核通过的图片
- 📤 **一键发布**: 发布到小红书、抖音、B站等平台

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/atop0914/image-platform.git
cd image-platform
```

### 2. 配置

编辑 `config/config.yaml`:

```yaml
server:
  port: "8081"

database:
  host: localhost
  port: 3306
  user: root
  password: your_password
  dbname: image_platform

imageGen:
  outputDir: "~/generated_images"
  logDir: "~/generated_images/logs"
  width: 1024
  height: 2048

# 平台配置 - API Key 从环境变量自动加载
platforms:
  siliconflow:
    name: "硅基流动"
    envKey: "SILICONFLOW_API_KEY"
    url: "https://api.siliconflow.cn/v1"
    model: "Kwai-Kolors/Kolors"
    enabled: true
    description: "Kolors 模型，性价比高"

  aliyun:
    name: "阿里云百炼"
    envKey: "ALIYUN_API_KEY"
    url: "https://dashscope.aliyuncs.com/api/v1"
    model: "wanx-v1"
    enabled: true
    description: "通义万相，国内稳定"

  openai:
    name: "OpenAI DALL-E 3"
    envKey: "OPENAI_API_KEY"
    url: "https://api.openai.com/v1"
    model: "dall-e-3"
    enabled: false
    description: "质量最高，需要国外支付"
```

### 3. 环境变量

在系统环境变量或 `.env` 文件中配置 API Key:

```bash
# 硅基流动
export SILICONFLOW_API_KEY='your-key'

# 阿里云百炼
export ALIYUN_API_KEY='your-key'

# OpenAI
export OPENAI_API_KEY='your-key'
```

### 4. 创建数据库

```sql
CREATE DATABASE image_platform;
```

### 5. 运行

```bash
# 编译
go build -o image-platform ./cmd/server

# 运行
./image-platform -c config/config.yaml
```

访问 http://localhost:8081

## API 接口

### 1. 平台列表

```bash
GET /api/platforms
```

响应：
```json
{
  "platforms": [
    {
      "id": "siliconflow",
      "name": "硅基流动",
      "model": "Kwai-Kolors/Kolors",
      "description": "Kolors 模型，性价比高",
      "enabled": true
    }
  ]
}
```

### 2. 生成图片

```bash
POST /api/generate
Content-Type: application/json

{
  "prompt": "A cute cat sitting on a chair",
  "platform": "siliconflow",  // 必选：siliconflow, aliyun, modelscope
  "size": "1920x1080",        // 可选：图片尺寸，如 "1920x1080", "2048x2048"
  "model": "Tongyi-MAI/Z-Image-Turbo"  // 可选：指定模型，覆盖默认模型
}
```

响应：
```json
{
  "message": "success",
  "filePath": "~/generated_images/2026-02-20/siliconflow/215654.png",
  "platform": "硅基流动",
  "model": "Kwai-Kolors/Kolors"
}
```

**支持的自定义模型：**

| 平台 | 可用模型 |
|------|----------|
| 魔塔社区 | `Tongyi-MAI/Z-Image-Turbo` (默认), `Qwen/Qwen-Image`, `MusePublic/489_ckpt_FLUX_1` |

### 3. 图片列表

```bash
GET /api/images?status=all  # all, pending, approved, rejected
```

### 4. 审核图片

```bash
POST /api/moderate
Content-Type: application/json

{
  "id": 1,
  "status": "approved",  // approved, rejected
  "note": "质量很好"
}
```

### 5. 当天图库

```bash
GET /api/gallery?date=2026-02-20
```

### 6. 发布图片

```bash
POST /api/publish
Content-Type: application/json

{
  "image_id": 1,
  "platforms": ["xiaohongshu", "douyin"],  // 发布到哪些平台
  "title": "标题",
  "content": "正文内容"
}
```

### 7. 每日报告

```bash
GET /api/report?date=2026-02-20
```

## 支持的平台

| 平台 | 模型 | 说明 |
|------|------|------|
| 硅基流动 | Kolors | 国内首选，性价比高 |
| 阿里云百炼 | 通义万相 (wanx-v1) | 国内稳定，阿里云 |
| 魔塔社区 | 通义万相Turbo (Z-Image-Turbo) | 免费额度，速度快 |
| OpenAI | DALL-E 3 | 质量最高 |

## 目录结构

```
image-platform/
├── cmd/server/main.go   # 主服务入口
├── config/              # 配置文件
├── internal/
│   └── publisher/       # 发布模块
├── web/                 # 前端资源
│   ├── templates/       # HTML 模板
│   ├── css/           # 样式
│   └── js/            # 脚本
├── go.mod
├── go.sum
└── image-platform      # 编译好的二进制
```

## Web 界面

- **首页** `/` - 待审核图片列表
- **添加** `/add` - 手动添加图片
- **审核** `/moderate/:id` - 审核详情
- **记录** `/records` - 审核历史
- **图库** `/gallery` - 当天通过的图片

## License

MIT
