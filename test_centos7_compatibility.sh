#!/bin/bash

echo "========================================="
echo "CentOS 7 Git 兼容性测试脚本"
echo "========================================="
echo ""

echo "1. 检查系统信息："
if [ -f /etc/centos-release ]; then
    echo "系统版本: $(cat /etc/centos-release)"
else
    echo "系统版本: $(uname -a)"
fi

echo ""
echo "2. 检查Git版本："
git --version

echo ""
echo "3. 测试Git基本命令（不使用-C参数）："
echo "创建临时测试目录..."
rm -rf /tmp/git_test
mkdir -p /tmp/git_test
cd /tmp/git_test

echo "初始化Git仓库..."
git init

echo "测试Git命令是否支持老版本..."
git config user.name "test"
git config user.email "test@example.com"
echo "test file" > test.txt
git add test.txt
git commit -m "test commit"

echo "创建测试分支..."
git checkout -b test-branch
echo "branch file" > branch.txt
git add branch.txt
git commit -m "branch commit"

echo "测试分支列表..."
git branch -a

echo "切换回主分支..."
git checkout master 2>/dev/null || git checkout main 2>/dev/null || echo "使用当前分支"

echo ""
echo "4. 清理测试目录..."
cd /
rm -rf /tmp/git_test

echo ""
echo "5. 运行备份工具测试（优化模式）："
echo "删除旧备份..."
rm -rf gitlab_backups/

echo ""
echo "开始测试备份工具（应该不再出现 'Unknown option: -C' 错误）："
echo "命令: ./back -o -b"
echo ""

./back -o -b

echo ""
echo "========================================="
echo "测试完成！"
echo ""

if [ -d "gitlab_backups" ]; then
    echo "✓ 备份目录创建成功"
    
    echo ""
    echo "检查分支信息文件："
    find gitlab_backups/ -name "branches_info.json" -exec echo "文件: {}" \; -exec head -20 {} \;
    
    echo ""
    echo "检查工作目录："
    find gitlab_backups/ -name "workspace" -type d | while read dir; do
        echo "工作目录: $dir"
        if [ -d "$dir" ]; then
            cd "$dir"
            echo "  当前分支: $(git branch 2>/dev/null | grep '*' | sed 's/* //')"
            echo "  所有分支: $(git branch -a 2>/dev/null | wc -l) 个"
            cd - > /dev/null
        fi
    done
else
    echo "✗ 备份目录未创建，可能存在问题"
fi

echo ""
echo "兼容性检查："
if grep -q "Unknown option: -C" gitlab_backups/*/reports/backup_report.txt 2>/dev/null; then
    echo "✗ 仍然存在 -C 参数兼容性问题"
else
    echo "✓ Git -C 参数兼容性问题已解决"
fi

echo ""
echo "测试总结："
echo "- 如果看到多个分支被checkout，说明分支检测正常"
echo "- 如果没有出现 'Unknown option: -C' 错误，说明兼容性修复成功"
echo "- 检查 branches_info.json 文件中应该包含多个分支信息"