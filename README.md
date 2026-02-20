# image-platform

图片生成与审核一体化平台 (Go + Gin + MySQL)

## 功能特性

- 🤖 **多平台图片生成**: 支持 SiliconFlow、OpenAI 等多种模型提供商
- 📥 **自动入库**: 生成图片自动添加到审核队列
- ✅ **人工审核**: Web 界面审核通过/拒绝
- 📊 **数据统计**: 每日审核报告 API

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/atop0914/image-platform.git
cd image-platform
```

### 2. 配置

复制配置示例并修改:

```bash
cp config/config.yaml config/config.yaml.bak
```

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
    enabled: false

  openai:
    name: "OpenAI DALL-E 3"
    envKey: "OPENAI_API_KEY"
    url: "https://api.openai.com/v1"
    model: "dall-e-3"
    enabled: false
```

### 3. 创建数据库

```sql
CREATE DATABASE image_platform;
```

### 4. 运行

```bash
# 方式一: 直接运行
export SILICONFLOW_API_KEY='your-api-key'
./image-platform -c config/config.yaml

# 方式二: Docker
docker run -d -p 8081:8081 \
  -e SILICONFLOW_API_KEY='your-api-key' \
  -v ./config:/app/config \
  -v ./generated_images:/app/generated_images \
  image-platform
```

### 5. 访问

- Web 界面: http://localhost:8081
- 首页: 待审核图片列表
- 添加: http://localhost:8081/add
- 审核: http://localhost:8081/moderate/:id
- 记录: http://localhost:8081/records

## API

### 生成图片

```bash
curl -X POST http://localhost:8081/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "一只可爱的橘猫",
    "platform": "siliconflow"
  }'
```

响应:
```json
{
  "message": "success",
  "filePath": "/home/user/generated_images/20260220_120000_一只可爱的橘猫/siliconflow_123456.png",
  "platform": "硅基流动"
}
```

### 图片列表

```bash
curl http://localhost:8081/api/images?status=all
```

### 审核图片

```bash
curl -X POST http://localhost:8081/api/moderate \
  -H "Content-Type: application/json" \
  -d '{
    "id": 1,
    "status": "approved",
    "note": "质量很好"
  }'
```

### 每日报告

```bash
curl http://localhost:8081/api/report?date=2026-02-20
```

响应:
```json
{
  "date": "2026-02-20",
  "total": 10,
  "approved": 7,
  "rejected": 2,
  "pending": 1,
  "images": [...]
}
```

## 支持的平台

| 平台 | 模型 | 说明 |
|------|------|------|
| SiliconFlow | Kolors | 国内首选，稳定 |
| OpenAI | DALL-E 3 | 质量最高 |

## 目录结构

```
image-platform/
├── cmd/server/main.go   # 主服务入口
├── config/              # 配置文件
├── internal/
│   ├── generator/       # 图片生成模块
│   └── moderation/      # 审核模块
├── web/                 # 前端资源
│   ├── templates/       # HTML 模板
│   ├── css/            # 样式
│   └── js/             # 脚本
├── go.mod
├── go.sum
└── image-platform       # 编译好的二进制
```

## 开发

```bash
# 编译
go build -o image-platform ./cmd/server

# 运行测试
go test ./...

# 代码格式化
go fmt ./...
```

## License

MIT
