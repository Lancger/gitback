# GitLab备份工具使用示例

## 基本使用

### 1. 传统模式备份（默认）
```bash
# 备份repo.txt中指定的仓库（完整镜像 + 所有分支代码）
./back

# 不提取分支代码，只备份Git仓库
./back -n

# 只提取已备份仓库的分支代码
./back -e
```

### 2. 优化模式备份（推荐）
```bash
# 使用优化模式备份（浅克隆 + 分支信息）
./back -o

# 优化模式 + 不提取分支代码（最轻量）
./back -o -n

# 优化模式 + 下载分支压缩包
./back -o -z
```

## 高级使用

### 3. 获取所有仓库列表
```bash
# 只获取所有仓库列表，保存到all_repos.txt
./back -l

# 获取所有仓库列表并备份指定仓库
./back -a

# 获取所有仓库列表并使用优化模式备份
./back -a -o
```

### 4. 组合使用示例
```bash
# 完整流程：获取仓库列表 + 优化备份 + 下载压缩包
./back -a -o -z

# 只下载分支压缩包（不备份仓库）
./back -zo

# 获取仓库列表 + 传统模式备份 + 提取分支代码
./back -a -b -e
```

## 实际场景示例

### 场景1：日常备份（节省空间）
```bash
# 使用优化模式，只保存分支信息
./back -o -n

# 输出目录结构：
# gitlab_backups/20241201/repositories/project_name/
# ├── workspace/           # 单一工作目录
# └── branches_info.json   # 分支元数据
```

### 场景2：完整备份（保留历史）
```bash
# 使用传统模式，保存完整镜像和所有分支代码
./back

# 输出目录结构：
# gitlab_backups/20241201/repositories/project_name/
# ├── repository.git/      # 完整镜像仓库
# └── branches/           # 所有分支的完整代码
#     ├── main/
#     ├── develop/
#     └── feature_*/
```

### 场景3：混合备份（最佳实践）
```bash
# 第一次：完整备份（每月一次）
./back

# 日常：优化备份（每日）
./back -o

# 重要发布：下载压缩包存档
./back -z
```

### 场景4：CI/CD环境备份
```bash
# 轻量级备份，适合自动化脚本
./back -o -n

# 定期完整备份
./back -a -o -z  # 获取最新仓库列表并备份
```

## 配置文件示例

### repo.txt 配置示例
```
# 合约项目
https://git.qq.top/2024/qqmng-web.git
https://git.qq.top/2024/qqweb.git
https://git.qq.top/2024/qqapp.git

# 综合项目  
https://git.qq.top/qq_backend/qqmng-big.git
https://git.qq.top/qq_frontend/qq-web.git

# 也可以使用项目ID
123
456
```

## 输出文件说明

### 备份报告文件
```
gitlab_backups/20241201/reports/
├── projects.json        # 项目详细信息（JSON格式）
├── projects.txt         # 项目列表（文本格式）
└── backup_report.txt    # 备份统计报告
```

### 备份报告内容示例
```
GitLab项目备份报告
==================================================
备份日期: 20241201
备份开始时间: 2024-12-01 10:00:00
备份结束时间: 2024-12-01 10:15:00
备份总耗时: 15m0s
备份项目总数: 8
备份分支总数: 45
备份标签总数: 12
成功提取代码的项目数: 8
下载的分支压缩包总数: 0
总存储大小: 245.67 MB
优化模式项目数: 8
传统模式项目数: 0
备份目录: gitlab_backups/20241201
==================================================

项目详细统计:
--------------------------------------------------
项目: 2024/qqmng-web
  分支数: 8
  标签数: 2
  代码提取: 成功
  分支压缩包: 0
  备份模式: 优化模式
  存储大小: 35.24 MB
--------------------------------------------------
```

### 分支信息文件示例 (branches_info.json)
```json
[
  {
    "name": "main",
    "last_commit": "a1b2c3d4e5f6789...",
    "last_update": "2024-12-01T10:30:00Z",
    "commit_count": 245
  },
  {
    "name": "develop", 
    "last_commit": "f6e5d4c3b2a1987...",
    "last_update": "2024-11-30T15:45:00Z",
    "commit_count": 189
  },
  {
    "name": "feature/new-ui",
    "last_commit": "9876543210abcde...",
    "last_update": "2024-11-29T09:20:00Z", 
    "commit_count": 156
  }
]
```

## 性能对比示例

### 测试环境
- 项目数量：10个
- 平均分支数：8个
- 平均项目大小：50MB

### 对比结果
```
模式对比：
┌──────────────┬─────────────┬─────────────┬─────────────┬─────────────┐
│   备份模式   │  下载时间   │  存储空间   │  网络传输   │  适用场景   │
├──────────────┼─────────────┼─────────────┼─────────────┼─────────────┤
│   传统模式   │    45分钟   │   2.5 GB    │   2.3 GB    │  完整备份   │
│   优化模式   │    12分钟   │   350 MB    │   280 MB    │  日常备份   │
│  压缩包模式  │    8分钟    │   180 MB    │   150 MB    │  存档备份   │
└──────────────┴─────────────┴─────────────┴─────────────┴─────────────┘

节省效果：
- 时间节省：73%
- 空间节省：86%  
- 传输节省：88%
```

## 故障排除

### 常见问题

1. **克隆失败**
```bash
# 检查网络连接和令牌
./back -l  # 先测试API访问

# 如果SSL证书问题
git config --global http.sslverify false
```

2. **空间不足**
```bash
# 使用优化模式减少空间占用
./back -o -n

# 清理旧备份
rm -rf gitlab_backups/older_dates/
```

3. **权限问题**
```bash
# 检查目录权限
ls -la gitlab_backups/
sudo chown -R $USER:$USER gitlab_backups/
```

4. **仓库URL格式错误**
```bash
# 检查repo.txt文件格式
cat repo.txt
# 确保URL格式正确：https://domain/group/project.git
```

## 最佳实践建议

1. **备份策略**：
   - 每日：优化模式备份 (`./back -o`)
   - 每周：包含压缩包 (`./back -o -z`)
   - 每月：完整备份 (`./back`)

2. **存储管理**：
   - 定期清理旧备份
   - 使用优化模式节省空间
   - 重要分支可创建快照

3. **监控建议**：
   - 监控备份报告中的失败项目
   - 定期检查存储空间使用
   - 验证备份文件完整性

4. **自动化脚本示例**：
```bash
#!/bin/bash
# daily_backup.sh

# 设置日志
LOG_FILE="backup_$(date +%Y%m%d).log"

# 执行优化备份
./back -o -n 2>&1 | tee $LOG_FILE

# 检查结果
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo "备份成功完成" >> $LOG_FILE
else
    echo "备份失败" >> $LOG_FILE
    # 发送告警邮件等
fi
```