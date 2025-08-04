#!/bin/bash

echo "=========================="
echo "分支检测测试脚本"
echo "=========================="
echo ""

echo "1. 删除旧的备份目录（如果存在）"
rm -rf gitlab_backups/

echo "2. 运行优化模式备份，查看分支检测情况"
echo "命令: ./back -o -b"
echo ""

echo "预期看到的日志："
echo "- 正在获取所有远程分支信息..."
echo "- 成功获取远程分支信息"
echo "- 正在获取远程分支列表..."
echo "- 远程分支命令输出: (显示所有分支)"
echo "- 项目 XXX 共发现 N 个分支 (N应该>1)"
echo "- 开始处理分支 1/N, 2/N, ... N/N"
echo "- 正在checkout分支: 各个分支名"
echo "- ✓ 成功checkout分支: 各个分支名"
echo ""

echo "现在运行测试："
echo "================================"
./back -o -b

echo ""
echo "================================"
echo "测试完成！"
echo ""
echo "检查结果："
echo "1. 查看分支信息文件："
find gitlab_backups/ -name "branches_info.json" -exec echo "文件: {}" \; -exec cat {} \;
echo ""
echo "2. 查看workspace目录中的分支："
find gitlab_backups/ -name "workspace" -type d -exec echo "工作目录: {}" \; -exec sh -c 'cd "$1" && git branch -a' _ {} \;