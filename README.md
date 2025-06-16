# GitLab仓库备份工具

这是一个用于备份GitLab仓库的工具，支持备份所有分支和标签，并可以将每个分支的代码提取到单独的目录中，方便直接访问和使用。还可以下载每个分支的代码压缩包，完全不依赖于GitLab服务。

## 功能特点

1. **完整备份** - 使用`git clone --mirror`命令备份仓库的所有分支和标签
2. **增量更新** - 对已存在的仓库进行增量更新，避免重复下载
3. **分支代码提取** - 将每个分支的代码提取到单独的目录中，方便直接访问
4. **分支压缩包** - 下载每个分支的代码压缩包，完全不依赖GitLab服务
5. **并发下载** - 支持并发下载多个仓库，提高备份效率
6. **详细报告** - 生成详细的备份报告，包括分支和标签的统计信息
7. **灵活配置** - 支持从文件中读取仓库列表，也可以获取GitLab上的所有仓库

## 安装

1. 确保已安装Go环境（Go 1.16或更高版本）
2. 克隆本仓库：`git clone https://your-repo-url.git`
3. 进入目录：`cd gitback`
4. 编译程序：`go build -o gitback`

## Windows打包

```bash
git clone https://git.qq.top/ai/aa.git
cd aa
go build -o gitback.exe back.go
```

也可以使用以下命令进行交叉编译，在其他系统上为Windows生成可执行文件：

```bash
# 在Linux/macOS上为Windows 64位系统编译
GOOS=windows GOARCH=amd64 go build -o gitback.exe back.go

# 在Linux/macOS上为Windows 32位系统编译
GOOS=windows GOARCH=386 go build -o gitback.exe back.go
```

## 配置

1. 编辑`back.go`文件，修改以下常量：
   - `GITLAB_URL` - GitLab实例地址
   - `PRIVATE_TOKEN` - GitLab私人访问令牌
   - `CONCURRENT` - 并发下载数量
   - `REPO_FILE` - 存储仓库URL的文件（默认为`repo.txt`）

2. 创建`repo.txt`文件，每行一个仓库URL或项目ID，例如：
   ```
   #项目组1
   https://gitlab.example.com/group1/project1.git
   https://gitlab.example.com/group1/project2.git
   
   #项目组2
   https://gitlab.example.com/group2/project3.git
   123  # 项目ID
   ```

## 使用方法

```
./gitback [选项]
```

### 选项

- 无参数 - 默认备份`repo.txt`中指定的仓库并提取分支代码
- `-l, --list` - 获取所有仓库列表并保存到`all_repos.txt`
- `-b, --backup` - 备份`repo.txt`中指定的仓库
- `-a, --all` - 获取所有仓库列表并备份`repo.txt`中的仓库
- `-n, --no-extract` - 不提取分支代码
- `-e, --extract-only` - 只提取已备份仓库的分支代码，不执行备份
- `-z, --archives` - 下载所有分支的压缩包
- `-zo, --archives-only` - 只下载分支压缩包，不执行备份和提取代码
- `-h, --help` - 显示帮助信息

### 示例

1. 备份指定的仓库并提取分支代码：
   ```
   ./gitback
   ```

2. 获取所有仓库列表但不备份：
   ```
   ./gitback --list
   ```

3. 备份仓库但不提取分支代码：
   ```
   ./gitback --no-extract
   ```

4. 只提取已备份仓库的分支代码：
   ```
   ./gitback --extract-only
   ```

5. 获取所有仓库列表并备份：
   ```
   ./gitback --all
   ```

6. 下载所有分支的压缩包：
   ```
   ./gitback --archives
   ```

7. 只下载分支压缩包，不执行备份和提取代码：
   ```
   ./gitback --archives-only
   ```

## 备份目录结构

备份文件将保存在`gitlab_backups/日期`目录下，结构如下：

```
gitlab_backups/
└── 20230101/                    # 按日期组织的备份目录
    ├── repositories/            # 仓库备份目录
    │   └── group/project/       # 按项目组织的目录
    │       ├── repository.git/  # Git镜像仓库（包含所有分支和标签）
    │       ├── branches/        # 提取的分支代码目录
    │       │   ├── master/      # master分支的代码
    │       │   ├── dev/         # dev分支的代码
    │       │   └── feature_x/   # 其他分支的代码
    │       └── archives/        # 分支压缩包目录
    │           ├── master.zip   # master分支的压缩包
    │           ├── dev.zip      # dev分支的压缩包
    │           └── feature_x.zip # 其他分支的压缩包
    └── reports/                 # 备份报告目录
        ├── backup_report.txt    # 备份报告（文本格式）
        └── projects.json        # 项目信息（JSON格式）
```

## 备份方式比较

本工具提供了三种备份方式，可以根据需要选择使用：

1. **Git镜像仓库** - 使用`git clone --mirror`命令备份，保留完整的Git历史记录和所有分支、标签
   - 优点：完整保留所有历史记录和元数据
   - 缺点：需要Git命令才能访问代码

2. **分支代码提取** - 将每个分支的代码提取到单独的目录
   - 优点：可以直接访问代码，不需要Git命令
   - 缺点：不保留Git历史记录

3. **分支压缩包** - 下载每个分支的代码压缩包
   - 优点：完全独立的备份，不依赖于Git，可以直接解压使用
   - 缺点：不保留Git历史记录，占用更多存储空间

## 备份报告

备份完成后，会在`reports`目录下生成详细的备份报告，包括：

- 备份日期和时间
- 备份总耗时
- 备份项目总数
- 备份分支总数
- 备份标签总数
- 成功提取代码的项目数
- 下载的分支压缩包总数
- 每个项目的详细统计信息

## 注意事项

1. 确保GitLab私人访问令牌具有足够的权限（至少需要`read_repository`权限）
2. 对于大型仓库，建议增加超时时间和并发数量
3. 定期清理旧的备份以节省磁盘空间
4. 下载分支压缩包可能会占用较多存储空间，请确保有足够的磁盘空间

## 许可证

[MIT License](LICENSE)
