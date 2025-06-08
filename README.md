# GitBack

GitLab仓库备份工具，支持备份指定仓库或获取所有仓库列表。

## 功能特点

- 备份指定的GitLab仓库（从repo.txt文件读取）
- 获取GitLab实例中的所有仓库列表并保存到文件
- 支持多种方式查找项目：项目ID、克隆URL、项目路径、项目名称
- 并发下载，提高备份效率
- 自动重试失败的下载
- 生成详细的备份报告

## 安装

### 前置条件

- Go 1.16或更高版本

### 编译

```bash
git clone https://git.qq.top/ai/aa.git
cd aa
go build -o gitback back.go
```

## 配置

在`back.go`文件中修改以下常量：

```go
const (
	GITLAB_URL    = "https://git.qq.top/"  // 替换为您的 GitLab 实例地址
	PRIVATE_TOKEN = "x7TfeZy49Ks3LT4Hx9bw" // 替换为您的私人令牌
	MAX_RETRIES   = 3                      // 最大重试次数
	CONCURRENT    = 5                      // 并发下载数
	REPO_FILE     = "repo.txt"             // 存储仓库URL的文件
	ALL_REPO_FILE = "all_repos.txt"        // 存储所有仓库URL的文件
)
```

## 使用方法

### 准备仓库列表

创建一个名为`repo.txt`的文件，每行包含一个要备份的仓库URL，例如：

```
https://git.qq.top/ai/aa.git
```

支持以下格式：
- 完整的HTTP(S) URL：`https://git.qq.top/group/project.git`
- 项目路径：`group/project`
- 项目ID：纯数字

### 命令行参数

```
GitLab仓库备份工具
用法:
  无参数     - 默认备份repo.txt中指定的仓库
  -l, --list - 获取所有仓库列表并保存到all_repos.txt
  -b, --backup - 备份repo.txt中指定的仓库
  -a, --all   - 获取所有仓库列表并备份repo.txt中的仓库
  -h, --help  - 显示此帮助信息
```

### 示例

1. 备份指定的仓库：

```bash
./gitback
```

2. 获取所有仓库列表：

```bash
./gitback -l
```

3. 获取所有仓库列表并备份指定仓库：

```bash
./gitback -a
```

4. 显示帮助信息：

```bash
./gitback -h
```

## 备份文件结构

备份文件将保存在`gitlab_backups/日期/`目录下，结构如下：

```
gitlab_backups/
└── 20250608/
    ├── repositories/
    │   └── group/
    │       └── project/
    │           └── repository.zip
    └── reports/
        ├── backup_report.txt
        ├── projects.json
        └── projects.txt
```

## 故障排除

1. 如果遇到API请求失败，请检查：
   - GitLab实例地址是否正确
   - 私人令牌是否有效
   - 网络连接是否正常

2. 如果无法找到项目，请尝试：
   - 检查repo.txt中的URL格式
   - 使用不同的项目标识（URL、路径、ID）
   - 确认项目在GitLab实例中存在

## 许可证

MIT
