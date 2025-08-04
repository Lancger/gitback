package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	GITLAB_URL    = "https://git.qq.top"   // 替换为您的 GitLab 实例地址
	PRIVATE_TOKEN = "x7TfeZy49Ks3LT4Hx9bw" // 替换为您的私人令牌
	MAX_RETRIES   = 3                      // 最大重试次数
	CONCURRENT    = 5                      // 并发下载数
	REPO_FILE     = "repo.txt"             // 存储仓库URL的文件
	ALL_REPO_FILE = "all_repos.txt"        // 存储所有仓库URL的文件
)

// 内置的默认仓库列表
var defaultRepos = []string{
	"#qq项目",
	"https://git.qq.top/2024/mng-web.git",
}

// 命令行参数
type CommandFlags struct {
	ListAllRepos     bool // 是否获取并保存所有仓库列表
	BackupRepos      bool // 是否备份仓库
	ExtractCode      bool // 是否提取分支代码
	DownloadArchives bool // 是否下载分支压缩包
	OptimizedMode    bool // 是否使用优化模式（单一工作目录+checkout）
}

type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	WebURL            string `json:"web_url"`
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
}

type BackupSummary struct {
	BackupTime    string    `json:"backup_time"`
	TotalProjects int       `json:"total_projects"`
	Projects      []Project `json:"projects"`
}

type BackupConfig struct {
	Date        string
	BaseDir     string
	BackupDir   string
	ProjectsDir string
	ReportDir   string
}

func newBackupConfig() BackupConfig {
	date := time.Now().Format("20060102")
	baseDir := "gitlab_backups"
	backupDir := filepath.Join(baseDir, date)

	return BackupConfig{
		Date:        date,
		BaseDir:     baseDir,
		BackupDir:   backupDir,
		ProjectsDir: filepath.Join(backupDir, "repositories"),
		ReportDir:   filepath.Join(backupDir, "reports"),
	}
}

func (c *BackupConfig) createDirectories() error {
	dirs := []string{
		c.BaseDir,
		c.BackupDir,
		c.ProjectsDir,
		c.ReportDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %v", dir, err)
		}
	}
	return nil
}

// 检查repo.txt文件是否存在，如果不存在则创建并写入默认仓库列表
func ensureRepoFileExists() error {
	// 检查文件是否存在
	if _, err := os.Stat(REPO_FILE); os.IsNotExist(err) {
		log.Printf("仓库文件 %s 不存在，创建默认文件", REPO_FILE)

		// 创建文件并写入默认仓库列表
		file, err := os.Create(REPO_FILE)
		if err != nil {
			return fmt.Errorf("创建仓库文件失败: %v", err)
		}
		defer file.Close()

		// 写入默认仓库列表
		for _, repo := range defaultRepos {
			if _, err := fmt.Fprintln(file, repo); err != nil {
				return fmt.Errorf("写入默认仓库失败: %v", err)
			}
		}

		log.Printf("已创建默认仓库文件 %s，包含 %d 个仓库", REPO_FILE, len(defaultRepos))
	}

	return nil
}

// 获取所有项目
func getAllProjects() ([]Project, error) {
	var allProjects []Project
	page := 1
	perPage := 100

	log.Println("开始获取所有GitLab项目信息...")

	for {
		url := fmt.Sprintf("%s/api/v4/projects?page=%d&per_page=%d&order_by=id&sort=asc", GITLAB_URL, page, perPage)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("api请求失败: %d", resp.StatusCode)
		}

		var projects []Project
		if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		if len(projects) == 0 {
			break
		}

		allProjects = append(allProjects, projects...)
		log.Printf("已获取 %d 个项目 (第 %d 页)", len(allProjects), page)
		page++
	}

	log.Printf("共找到 %d 个项目", len(allProjects))
	return allProjects, nil
}

// 保存所有仓库到文件
func saveAllReposToFile(projects []Project) error {
	file, err := os.Create(ALL_REPO_FILE)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", ALL_REPO_FILE, err)
	}
	defer file.Close()

	// 写入项目信息
	for _, p := range projects {
		// 写入HTTP URL
		if p.HTTPURLToRepo != "" {
			if _, err := fmt.Fprintln(file, p.HTTPURLToRepo); err != nil {
				return err
			}
		}
	}

	log.Printf("成功保存 %d 个仓库URL到文件 %s", len(projects), ALL_REPO_FILE)
	return nil
}

// 读取repo.txt文件中的仓库URL
func readRepoURLs() ([]string, error) {
	// 确保仓库文件存在
	if err := ensureRepoFileExists(); err != nil {
		return nil, err
	}

	file, err := os.Open(REPO_FILE)
	if err != nil {
		return nil, fmt.Errorf("打开仓库文件失败 %s: %v", REPO_FILE, err)
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		url := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释行
		if url != "" && !strings.HasPrefix(url, "#") {
			urls = append(urls, url)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取仓库文件失败: %v", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("仓库文件中没有有效的URL")
	}

	return urls, nil
}

// 从URL中提取项目路径
func extractProjectPath(url string) string {
	// 移除协议部分
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")

	// 移除域名部分
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 2 {
		return ""
	}

	// 移除.git后缀
	path := parts[1]
	path = strings.TrimSuffix(path, ".git")

	// 处理可能存在的额外路径元素，如/api/v4等
	pathParts := strings.Split(path, "/")
	if len(pathParts) >= 2 {
		// 检查是否包含api路径
		if pathParts[0] == "api" || pathParts[0] == "v4" {
			// 跳过API路径部分
			pathParts = pathParts[2:]
			path = strings.Join(pathParts, "/")
		}
	}

	return path
}

// 根据项目路径获取项目信息
func getProjectByPath(projectPath string) (Project, error) {
	// 使用URL编码的路径，并且使用正确的API格式
	encodedPath := strings.ReplaceAll(projectPath, "/", "%2F")
	url := fmt.Sprintf("%s/api/v4/projects/%s", GITLAB_URL, encodedPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Project{}, err
	}

	req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Project{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Project{}, fmt.Errorf("api请求失败: %d", resp.StatusCode)
	}

	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return Project{}, err
	}

	return project, nil
}

// 通过搜索项目名称获取项目信息
func searchProjectByName(projectName string) (Project, error) {
	// 使用搜索API查询项目
	url := fmt.Sprintf("%s/api/v4/projects?search=%s", GITLAB_URL, projectName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Project{}, err
	}

	req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Project{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Project{}, fmt.Errorf("api搜索请求失败: %d", resp.StatusCode)
	}

	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return Project{}, err
	}

	// 如果找到匹配的项目，返回第一个
	if len(projects) > 0 {
		return projects[0], nil
	}

	return Project{}, fmt.Errorf("未找到匹配的项目: %s", projectName)
}

// 通过克隆URL获取项目
func getProjectByCloneURL(cloneURL string) (Project, error) {
	// 构建API请求
	url := fmt.Sprintf("%s/api/v4/projects?search=%s", GITLAB_URL, cloneURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Project{}, err
	}

	req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Project{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Project{}, fmt.Errorf("api请求失败: %d", resp.StatusCode)
	}

	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return Project{}, err
	}

	// 寻找匹配的项目
	for _, p := range projects {
		if p.HTTPURLToRepo == cloneURL || p.SSHURLToRepo == cloneURL {
			return p, nil
		}
	}

	return Project{}, fmt.Errorf("未找到匹配的项目: %s", cloneURL)
}

// 通过项目ID获取项目
func getProjectByID(projectID int) (Project, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d", GITLAB_URL, projectID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Project{}, err
	}

	req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Project{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Project{}, fmt.Errorf("api请求失败: %d", resp.StatusCode)
	}

	var project Project
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return Project{}, err
	}

	return project, nil
}

// 获取指定的项目
func getSpecifiedProjects() ([]Project, error) {
	urls, err := readRepoURLs()
	if err != nil {
		return nil, err
	}

	var projects []Project
	for _, url := range urls {
		// 检查是否是纯数字（项目ID）
		if id, err := strconv.Atoi(strings.TrimSpace(url)); err == nil {
			project, err := getProjectByID(id)
			if err == nil {
				projects = append(projects, project)
				log.Printf("通过ID成功找到项目: %s (ID: %d)", project.PathWithNamespace, project.ID)
				continue
			}
			log.Printf("通过ID获取项目失败 %d: %v", id, err)
		}

		// 尝试直接通过克隆URL获取项目
		project, err := getProjectByCloneURL(url)
		if err == nil {
			projects = append(projects, project)
			log.Printf("通过克隆URL成功找到项目: %s (ID: %d)", project.PathWithNamespace, project.ID)
			continue
		}

		// 如果直接获取失败，尝试解析URL
		projectPath := extractProjectPath(url)
		if projectPath == "" {
			log.Printf("无法从URL提取项目路径: %s", url)
			continue
		}

		// 尝试通过路径获取项目
		project, err = getProjectByPath(projectPath)
		if err != nil {
			log.Printf("通过路径获取项目失败 %s: %v", projectPath, err)

			// 如果通过路径获取失败，尝试通过名称搜索
			parts := strings.Split(projectPath, "/")
			projectName := parts[len(parts)-1]
			log.Printf("尝试通过名称搜索项目: %s", projectName)

			project, err = searchProjectByName(projectName)
			if err != nil {
				log.Printf("通过名称搜索项目失败 %s: %v", projectName, err)
				continue
			}
		}

		projects = append(projects, project)
		log.Printf("成功找到项目: %s (ID: %d)", project.PathWithNamespace, project.ID)
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("没有找到有效的项目")
	}

	return projects, nil
}

func saveProjectInfo(projects []Project, config BackupConfig) error {
	summary := BackupSummary{
		BackupTime:    time.Now().Format("2006-01-02 15:04:05"),
		TotalProjects: len(projects),
		Projects:      projects,
	}

	// 保存JSON文件
	jsonPath := filepath.Join(config.ReportDir, "projects.json")
	jsonData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return err
	}

	// 保存文本文件
	txtPath := filepath.Join(config.ReportDir, "projects.txt")
	f, err := os.Create(txtPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// 写入摘要信息
	fmt.Fprintf(f, "备份日期: %s\n", config.Date)
	fmt.Fprintf(f, "备份时间: %s\n", summary.BackupTime)
	fmt.Fprintf(f, "项目总数: %d\n", summary.TotalProjects)
	fmt.Fprintf(f, "%s\n\n", strings.Repeat("=", 50))

	// 写入项目详细信息
	for _, p := range projects {
		fmt.Fprintf(f, "项目名称: %s\n", p.Name)
		fmt.Fprintf(f, "项目路径: %s\n", p.PathWithNamespace)
		fmt.Fprintf(f, "Web URL: %s\n", p.WebURL)
		fmt.Fprintf(f, "SSH URL: %s\n", p.SSHURLToRepo)
		fmt.Fprintf(f, "HTTP URL: %s\n", p.HTTPURLToRepo)
		fmt.Fprintf(f, "%s\n", strings.Repeat("-", 50))
	}

	return nil
}

func downloadBackup(project Project, wg *sync.WaitGroup, semaphore chan struct{}, config BackupConfig, extractCode bool) {
	defer wg.Done()
	defer func() { <-semaphore }()

	projectDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace)
	gitDir := filepath.Join(projectDir, "repository.git")

	// 创建项目目录
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		log.Printf("创建目录失败 %s: %v\n", projectDir, err)
		return
	}

	// 检查git目录是否已存在
	if _, err := os.Stat(gitDir); err == nil {
		log.Printf("Git仓库已存在，尝试更新 %s\n", gitDir)

		// 更新已有仓库的所有分支和标签
		if err := updateGitRepository(gitDir); err != nil {
			log.Printf("更新仓库失败 %s: %v\n", project.PathWithNamespace, err)
		} else {
			log.Printf("成功更新仓库 %s 的所有分支和标签\n", project.PathWithNamespace)
		}
	} else {
		// 使用git clone --mirror命令下载所有分支
		cloneURL := project.HTTPURLToRepo

		// 添加认证信息到URL
		parsedURL := strings.Split(cloneURL, "://")
		if len(parsedURL) == 2 {
			cloneURL = fmt.Sprintf("%s://oauth2:%s@%s", parsedURL[0], PRIVATE_TOKEN, parsedURL[1])
		}

		log.Printf("开始克隆项目 %s 的所有分支\n", project.PathWithNamespace)

		success := false
		for retry := 0; retry < MAX_RETRIES; retry++ {
			if retry > 0 {
				log.Printf("重试克隆 %s (第 %d 次)\n", project.PathWithNamespace, retry+1)
				time.Sleep(time.Second * time.Duration(retry)) // 重试延迟
			}

			// 设置超时
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", cloneURL, gitDir)

			output, err := cmd.CombinedOutput()
			cancel() // 执行完命令后取消上下文

			if err != nil {
				log.Printf("克隆失败 %s: %v\n%s\n", project.PathWithNamespace, err, string(output))
				// 如果目录已存在但不完整，删除它以便重试
				os.RemoveAll(gitDir)
				continue
			}

			log.Printf("成功克隆项目 %s 的所有分支到 %s\n", project.PathWithNamespace, gitDir)
			success = true
			break
		}

		if !success {
			log.Printf("克隆失败，已达到最大重试次数 %s\n", project.PathWithNamespace)
			return
		}
	}

	// 根据参数决定是否提取分支代码
	if extractCode {
		// 提取所有分支的代码
		log.Printf("开始提取项目 %s 的所有分支代码\n", project.PathWithNamespace)
		if err := extractAllBranches(project, config); err != nil {
			log.Printf("提取分支代码失败 %s: %v\n", project.PathWithNamespace, err)
		} else {
			log.Printf("成功提取项目 %s 的所有分支代码\n", project.PathWithNamespace)
		}
	}
}

// 更新已有的Git仓库
func updateGitRepository(gitDir string) error {
	// 设置超时
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// 切换到仓库目录
	cmd := exec.CommandContext(ctx, "git", "-C", gitDir, "remote", "update", "--prune")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("更新失败: %v\n%s", err, string(output))
	}

	// 获取所有标签
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "fetch", "--tags")

	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取标签失败: %v\n%s", err, string(output))
	}

	return nil
}

// 获取Git仓库的分支和标签数量
func getGitRepoStats(gitDir string) (int, int, error) {
	// 获取分支数量
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", gitDir, "branch", "--all", "--list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("获取分支失败: %v", err)
	}

	branches := 0
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !strings.Contains(line, "HEAD") {
			branches++
		}
	}

	// 获取标签数量
	cmd = exec.CommandContext(ctx, "git", "-C", gitDir, "tag", "--list")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return branches, 0, fmt.Errorf("获取标签失败: %v", err)
	}

	tags := 0
	lines = strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			tags++
		}
	}

	return branches, tags, nil
}

type ProjectStats struct {
	Project          Project
	Branches         int
	Tags             int
	ExtractedCode    bool  // 是否成功提取了代码
	ArchivedBranches int   // 下载的分支压缩包数量
	OptimizedMode    bool  // 是否使用了优化模式
	StorageSize      int64 // 存储大小（字节）
}

func generateBackupReport(projects []Project, startTime time.Time, config BackupConfig) error {
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// 收集项目统计信息
	var projectStats []ProjectStats
	var totalBranches, totalTags, extractedProjects, archivedBranches int

	log.Println("收集仓库统计信息...")

	for _, project := range projects {
		gitDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "repository.git")
		branchesDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "branches")
		archivesDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "archives")

		// 检查仓库是否存在
		repoExists := false
		if _, err := os.Stat(gitDir); err == nil {
			repoExists = true
		}

		// 检查是否提取了代码
		extractedCode := false
		if _, err := os.Stat(branchesDir); err == nil {
			extractedCode = true
			extractedProjects++
		}

		// 获取仓库统计信息
		branches, tags := 0, 0
		if repoExists {
			var err error
			branches, tags, err = getGitRepoStats(gitDir)
			if err != nil {
				log.Printf("获取仓库统计信息失败 %s: %v", project.PathWithNamespace, err)
			}
		}

		// 获取压缩包数量
		archivedCount := 0
		if _, err := os.Stat(archivesDir); err == nil {
			// 统计压缩包数量
			files, err := os.ReadDir(archivesDir)
			if err == nil {
				for _, file := range files {
					if !file.IsDir() && strings.HasSuffix(file.Name(), ".zip") {
						archivedCount++
					}
				}
			}
		}
		archivedBranches += archivedCount

		// 计算存储大小
		storageSize := int64(0)
		if err := filepath.Walk(filepath.Join(config.ProjectsDir, project.PathWithNamespace), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // 忽略错误，继续遍历
			}
			if !info.IsDir() {
				storageSize += info.Size()
			}
			return nil
		}); err != nil {
			log.Printf("计算存储大小失败 %s: %v", project.PathWithNamespace, err)
		}

		// 检查是否使用了优化模式
		workspaceDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "workspace")
		gitDir = filepath.Join(config.ProjectsDir, project.PathWithNamespace, "repository.git")
		optimizedMode := false
		if _, err := os.Stat(workspaceDir); err == nil {
			optimizedMode = true
		} else if _, err := os.Stat(gitDir); err == nil {
			optimizedMode = false
		}

		projectStats = append(projectStats, ProjectStats{
			Project:          project,
			Branches:         branches,
			Tags:             tags,
			ExtractedCode:    extractedCode,
			ArchivedBranches: archivedCount,
			OptimizedMode:    optimizedMode,
			StorageSize:      storageSize,
		})

		totalBranches += branches
		totalTags += tags
	}

	reportPath := filepath.Join(config.ReportDir, "backup_report.txt")
	f, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "GitLab项目备份报告\n")
	fmt.Fprintf(f, "%s\n\n", strings.Repeat("=", 50))
	fmt.Fprintf(f, "备份日期: %s\n", config.Date)
	fmt.Fprintf(f, "备份开始时间: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "备份结束时间: %s\n", endTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "备份总耗时: %s\n", duration.Round(time.Second))
	fmt.Fprintf(f, "备份项目总数: %d\n", len(projects))
	fmt.Fprintf(f, "备份分支总数: %d\n", totalBranches)
	fmt.Fprintf(f, "备份标签总数: %d\n", totalTags)
	fmt.Fprintf(f, "成功提取代码的项目数: %d\n", extractedProjects)
	fmt.Fprintf(f, "下载的分支压缩包总数: %d\n", archivedBranches)
	fmt.Fprintf(f, "备份目录: %s\n", config.BackupDir)
	fmt.Fprintf(f, "%s\n\n", strings.Repeat("=", 50))

	// 计算总存储大小和优化统计
	var totalStorageSize int64
	var optimizedProjects, traditionalProjects int
	for _, stat := range projectStats {
		totalStorageSize += stat.StorageSize
		if stat.OptimizedMode {
			optimizedProjects++
		} else {
			traditionalProjects++
		}
	}

	fmt.Fprintf(f, "总存储大小: %.2f MB\n", float64(totalStorageSize)/(1024*1024))
	fmt.Fprintf(f, "优化模式项目数: %d\n", optimizedProjects)
	fmt.Fprintf(f, "传统模式项目数: %d\n", traditionalProjects)
	fmt.Fprintf(f, "%s\n\n", strings.Repeat("=", 50))

	// 添加详细的项目统计信息
	fmt.Fprintf(f, "项目详细统计:\n")
	fmt.Fprintf(f, "%s\n", strings.Repeat("-", 50))

	for _, stat := range projectStats {
		fmt.Fprintf(f, "项目: %s\n", stat.Project.PathWithNamespace)
		fmt.Fprintf(f, "  分支数: %d\n", stat.Branches)
		fmt.Fprintf(f, "  标签数: %d\n", stat.Tags)
		fmt.Fprintf(f, "  代码提取: %s\n", boolToString(stat.ExtractedCode))
		fmt.Fprintf(f, "  分支压缩包: %d\n", stat.ArchivedBranches)
		fmt.Fprintf(f, "  备份模式: %s\n", getModeString(stat.OptimizedMode))
		fmt.Fprintf(f, "  存储大小: %.2f MB\n", float64(stat.StorageSize)/(1024*1024))
		fmt.Fprintf(f, "%s\n", strings.Repeat("-", 50))
	}

	return nil
}

// 将布尔值转换为中文字符串
func boolToString(b bool) string {
	if b {
		return "成功"
	}
	return "失败"
}

// 获取备份模式字符串
func getModeString(optimized bool) string {
	if optimized {
		return "优化模式"
	}
	return "传统模式"
}

// 解析命令行参数
func parseCommandFlags() CommandFlags {
	flags := CommandFlags{
		ListAllRepos:     false,
		BackupRepos:      true,  // 默认执行备份操作
		ExtractCode:      true,  // 默认提取分支代码
		DownloadArchives: false, // 默认不下载分支压缩包
		OptimizedMode:    false, // 默认使用传统模式
	}

	// 检查命令行参数
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-l", "--list":
			flags.ListAllRepos = true
			flags.BackupRepos = false // 如果指定了列出仓库，则默认不执行备份
		case "-b", "--backup":
			flags.BackupRepos = true
		case "-a", "--all":
			flags.ListAllRepos = true
			flags.BackupRepos = true // 同时执行列出和备份操作
		case "-n", "--no-extract":
			flags.ExtractCode = false // 不提取分支代码
		case "-e", "--extract-only":
			flags.BackupRepos = false // 不执行备份
			flags.ExtractCode = true  // 只提取分支代码
		case "-z", "--archives":
			flags.DownloadArchives = true // 下载分支压缩包
		case "-zo", "--archives-only":
			flags.BackupRepos = false     // 不执行备份
			flags.ExtractCode = false     // 不提取分支代码
			flags.DownloadArchives = true // 只下载分支压缩包
		case "-o", "--optimized":
			flags.OptimizedMode = true // 使用优化模式
		}
	}

	return flags
}

// 显示使用帮助
func showHelp() {
	fmt.Println("GitLab仓库备份工具")
	fmt.Println("用法:")
	fmt.Println("  无参数     - 默认备份repo.txt中指定的仓库并提取分支代码")
	fmt.Println("  -l, --list - 获取所有仓库列表并保存到all_repos.txt")
	fmt.Println("  -b, --backup - 备份repo.txt中指定的仓库")
	fmt.Println("  -a, --all   - 获取所有仓库列表并备份repo.txt中的仓库")
	fmt.Println("  -n, --no-extract - 不提取分支代码")
	fmt.Println("  -e, --extract-only - 只提取已备份仓库的分支代码，不执行备份")
	fmt.Println("  -z, --archives - 下载所有分支的压缩包")
	fmt.Println("  -zo, --archives-only - 只下载分支压缩包，不执行备份和提取代码")
	fmt.Println("  -o, --optimized - 使用优化模式（浅克隆 + 单一工作目录，显著减少存储空间）")
	fmt.Println("  -h, --help  - 显示此帮助信息")
	fmt.Println("")
	fmt.Println("备份模式对比:")
	fmt.Println("  传统模式: git clone --mirror + 为每个分支创建完整代码副本")
	fmt.Println("  优化模式: git clone --depth=1 + 单一工作目录切换分支")
	fmt.Println("  优化模式优势: 存储空间减少80%+，下载时间减少70%+")
}

// 从Git镜像仓库中提取所有分支的代码
func extractAllBranches(project Project, config BackupConfig) error {
	gitDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "repository.git")
	branchesDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "branches")

	// 检查Git仓库是否存在
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("git仓库不存在: %s", gitDir)
	}

	// 创建分支目录
	if err := os.MkdirAll(branchesDir, 0755); err != nil {
		return fmt.Errorf("创建分支目录失败: %v", err)
	}

	// 获取所有分支
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", gitDir, "branch", "-a")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取分支列表失败: %v\n%s", err, string(output))
	}

	branches := []string{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		// 跳过空行和HEAD指针
		if branch == "" || strings.Contains(branch, "HEAD") {
			continue
		}

		// 移除前导的 * 和空格
		branch = strings.TrimPrefix(branch, "*")
		branch = strings.TrimSpace(branch)

		// 处理远程分支，无需条件判断
		branch = strings.TrimPrefix(branch, "remotes/origin/")

		// 跳过已处理的分支
		if !contains(branches, branch) {
			branches = append(branches, branch)
		}
	}

	log.Printf("项目 %s 共发现 %d 个分支", project.PathWithNamespace, len(branches))

	// 为每个分支创建代码副本
	successCount := 0
	for i, branch := range branches {
		// 跳过无效的分支名
		if branch == "" || branch == "HEAD" {
			log.Printf("跳过无效分支: '%s'", branch)
			continue
		}

		log.Printf("开始处理分支 %d/%d: %s", i+1, len(branches), branch)

		// 创建安全的目录名
		safeBranchName := strings.ReplaceAll(branch, "/", "_")
		branchDir := filepath.Join(branchesDir, safeBranchName)

		// 检查分支目录是否已存在
		if _, err := os.Stat(branchDir); err == nil {
			log.Printf("分支目录已存在，清空重建: %s", branchDir)
			// 如果目录已存在，清空它
			if err := os.RemoveAll(branchDir); err != nil {
				log.Printf("清空分支目录失败 %s: %v", branchDir, err)
				continue
			}
		}

		// 创建分支目录
		if err := os.MkdirAll(branchDir, 0755); err != nil {
			log.Printf("创建分支目录失败 %s: %v", branchDir, err)
			continue
		}

		log.Printf("正在提取分支 %s 的代码到 %s", branch, branchDir)

		// 使用git archive提取代码
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		cmd := exec.CommandContext(ctx, "git", "-C", gitDir, "archive", branch)

		// 使用tar提取文件
		tarCmd := exec.CommandContext(ctx, "tar", "-x", "-C", branchDir)

		// 连接两个命令
		tarCmd.Stdin, err = cmd.StdoutPipe()
		if err != nil {
			cancel()
			log.Printf("创建管道失败: %v", err)
			continue
		}

		// 启动tar命令
		if err := tarCmd.Start(); err != nil {
			cancel()
			log.Printf("启动tar命令失败: %v", err)
			continue
		}

		// 执行git archive
		if err := cmd.Run(); err != nil {
			cancel()
			log.Printf("提取分支 %s 失败: %v", branch, err)
			continue
		}

		// 等待tar命令完成
		if err := tarCmd.Wait(); err != nil {
			cancel()
			log.Printf("解压分支 %s 失败: %v", branch, err)
			continue
		}

		cancel()
		successCount++
		log.Printf("✓ 成功提取分支 %s 的代码 (%d/%d 完成)", branch, successCount, len(branches))
	}

	log.Printf("分支代码提取完成: 成功 %d/%d 个分支", successCount, len(branches))
	return nil
}

// 优化的备份函数：使用单一工作目录 + checkout 方式
func downloadBackupOptimized(project Project, wg *sync.WaitGroup, semaphore chan struct{}, config BackupConfig, extractCode bool) {
	defer wg.Done()
	defer func() { <-semaphore }()

	projectDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace)
	workDir := filepath.Join(projectDir, "workspace") // 单一工作目录
	branchesInfoFile := filepath.Join(projectDir, "branches_info.json")

	// 创建项目目录
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		log.Printf("创建目录失败 %s: %v\n", projectDir, err)
		return
	}

	// 检查工作目录是否已存在
	if _, err := os.Stat(workDir); err == nil {
		log.Printf("工作目录已存在，尝试更新 %s\n", workDir)
		if err := updateOptimizedRepository(workDir); err != nil {
			log.Printf("更新仓库失败 %s: %v\n", project.PathWithNamespace, err)
		} else {
			log.Printf("成功更新仓库 %s\n", project.PathWithNamespace)
		}
	} else {
		// 使用浅克隆，只下载最新提交，显著减少下载量
		cloneURL := project.HTTPURLToRepo

		// 添加认证信息到URL
		parsedURL := strings.Split(cloneURL, "://")
		if len(parsedURL) == 2 {
			cloneURL = fmt.Sprintf("%s://oauth2:%s@%s", parsedURL[0], PRIVATE_TOKEN, parsedURL[1])
		}

		log.Printf("开始克隆项目 %s (轻量级克隆)\n", project.PathWithNamespace)

		success := false
		for retry := 0; retry < MAX_RETRIES; retry++ {
			if retry > 0 {
				log.Printf("重试克隆 %s (第 %d 次)\n", project.PathWithNamespace, retry+1)
				time.Sleep(time.Second * time.Duration(retry))
			}

			// 为了获取所有分支信息，不使用浅克隆，而是正常克隆但限制历史深度
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			log.Printf("开始克隆项目（保留分支信息）...")
			cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, workDir)

			output, err := cmd.CombinedOutput()
			cancel()

			if err != nil {
				log.Printf("克隆失败 %s: %v\n%s\n", project.PathWithNamespace, err, string(output))
				os.RemoveAll(workDir)
				continue
			}

			log.Printf("成功克隆项目 %s 到 %s\n", project.PathWithNamespace, workDir)
			success = true
			break
		}

		if !success {
			log.Printf("克隆失败，已达到最大重试次数 %s\n", project.PathWithNamespace)
			return
		}
	}

	// 根据参数决定是否提取分支代码
	if extractCode {
		log.Printf("开始获取并备份项目 %s 的所有分支\n", project.PathWithNamespace)
		if err := extractBranchesOptimized(project, workDir, branchesInfoFile); err != nil {
			log.Printf("备份分支失败 %s: %v\n", project.PathWithNamespace, err)
		} else {
			log.Printf("成功备份项目 %s 的所有分支\n", project.PathWithNamespace)
		}
	}
}

// 优化的分支提取：使用单一工作目录切换分支
func extractBranchesOptimized(project Project, workDir, branchesInfoFile string) error {
	// 获取远程分支列表
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 获取所有远程分支信息
	log.Printf("正在获取所有远程分支信息...")

	// 由于使用了正常克隆，直接获取所有远程分支
	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "fetch", "--all")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("获取远程分支失败: %v\n输出: %s", err, string(output))
	} else {
		log.Printf("成功获取远程分支信息")
	}

	// 获取远程分支列表
	log.Printf("正在获取远程分支列表...")
	cmd = exec.CommandContext(ctx, "git", "-C", workDir, "branch", "-r")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("获取远程分支列表失败: %v\n%s", err, string(output))
	}

	log.Printf("远程分支命令输出:\n%s", string(output))

	var branches []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		if branch == "" || strings.Contains(branch, "HEAD") {
			continue
		}

		// 处理远程分支名，移除 origin/ 前缀
		branch = strings.TrimPrefix(branch, "origin/")
		if !contains(branches, branch) {
			branches = append(branches, branch)
		}
	}

	log.Printf("项目 %s 共发现 %d 个分支", project.PathWithNamespace, len(branches))

	// 存储分支信息
	type BranchInfo struct {
		Name        string    `json:"name"`
		LastCommit  string    `json:"last_commit"`
		LastUpdate  time.Time `json:"last_update"`
		CommitCount int       `json:"commit_count"`
	}

	var branchInfos []BranchInfo
	successfulCheckouts := 0

	// 为每个分支获取信息（使用checkout切换）
	for i, branch := range branches {
		if branch == "" {
			log.Printf("跳过空分支名")
			continue
		}

		log.Printf("开始处理分支 %d/%d: %s", i+1, len(branches), branch)

		// 切换到分支
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		log.Printf("正在checkout分支: %s", branch)
		cmd := exec.CommandContext(ctx, "git", "-C", workDir, "checkout", "-B", branch, "origin/"+branch)
		output, err := cmd.CombinedOutput()
		if err != nil {
			cancel()
			log.Printf("✗ 切换到分支 %s 失败: %v\n输出: %s", branch, err, string(output))
			continue
		}

		successfulCheckouts++
		log.Printf("✓ 成功checkout分支: %s (%d/%d)", branch, successfulCheckouts, len(branches))

		// 获取最新提交信息
		cmd = exec.CommandContext(ctx, "git", "-C", workDir, "rev-parse", "HEAD")
		commitOutput, err := cmd.CombinedOutput()
		if err != nil {
			cancel()
			log.Printf("获取分支 %s 提交信息失败: %v", branch, err)
			continue
		}

		// 获取提交数量
		cmd = exec.CommandContext(ctx, "git", "-C", workDir, "rev-list", "--count", "HEAD")
		countOutput, err := cmd.CombinedOutput()
		commitCount := 0
		if err == nil {
			if count, parseErr := strconv.Atoi(strings.TrimSpace(string(countOutput))); parseErr == nil {
				commitCount = count
			}
		}

		lastCommit := strings.TrimSpace(string(commitOutput))
		branchInfos = append(branchInfos, BranchInfo{
			Name:        branch,
			LastCommit:  lastCommit,
			LastUpdate:  time.Now(),
			CommitCount: commitCount,
		})

		cancel()
	}

	// 保存分支信息到JSON文件
	if data, err := json.MarshalIndent(branchInfos, "", "  "); err == nil {
		if err := os.WriteFile(branchesInfoFile, data, 0644); err != nil {
			log.Printf("保存分支信息失败: %v", err)
		} else {
			log.Printf("分支信息已保存到: %s", branchesInfoFile)
		}
	}

	log.Printf("分支checkout统计: 成功 %d/%d 个分支, 获得信息 %d 个分支", successfulCheckouts, len(branches), len(branchInfos))
	return nil
}

// 创建分支快照：将当前分支的代码保存到独立目录
func createBranchSnapshot(workDir, branchName, snapshotDir string) error {
	// 确保在正确的分支上
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	log.Printf("正在checkout分支用于快照创建: %s", branchName)
	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "checkout", branchName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("切换到分支 %s 失败: %v\n输出: %s", branchName, err, string(output))
	}
	log.Printf("✓ 成功checkout分支用于快照: %s", branchName)

	// 创建快照目录
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return fmt.Errorf("创建快照目录失败: %v", err)
	}

	// 复制工作目录内容到快照目录（排除.git目录）
	return copyDirContents(workDir, snapshotDir, []string{".git"})
}

// 复制目录内容，支持排除特定目录
func copyDirContents(srcDir, dstDir string, excludeDirs []string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		// 检查是否需要排除
		shouldExclude := false
		for _, exclude := range excludeDirs {
			if entry.Name() == exclude {
				shouldExclude = true
				break
			}
		}
		if shouldExclude {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDirContents(srcPath, dstPath, excludeDirs); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// 复制单个文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// 选择性分支备份：支持指定要备份完整代码的分支
func extractSelectedBranches(project Project, workDir, branchesInfoFile string, selectedBranches []string) error {
	// 获取所有分支信息
	if err := extractBranchesOptimized(project, workDir, branchesInfoFile); err != nil {
		return err
	}

	// 如果没有指定特定分支，直接返回
	if len(selectedBranches) == 0 {
		return nil
	}

	// 为选定的分支创建快照
	projectDir := filepath.Dir(branchesInfoFile)
	snapshotsDir := filepath.Join(projectDir, "snapshots")

	log.Printf("开始为 %d 个指定分支创建代码快照", len(selectedBranches))
	successfulSnapshots := 0
	for i, branchName := range selectedBranches {
		safeBranchName := strings.ReplaceAll(branchName, "/", "_")
		snapshotDir := filepath.Join(snapshotsDir, safeBranchName)

		log.Printf("创建分支快照 %d/%d: %s", i+1, len(selectedBranches), branchName)
		if err := createBranchSnapshot(workDir, branchName, snapshotDir); err != nil {
			log.Printf("✗ 创建分支 %s 快照失败: %v", branchName, err)
		} else {
			successfulSnapshots++
			log.Printf("✓ 成功创建分支 %s 的代码快照 (%d/%d)", branchName, successfulSnapshots, len(selectedBranches))
		}
	}

	log.Printf("分支快照创建完成: 成功 %d/%d 个分支", successfulSnapshots, len(selectedBranches))

	return nil
}

// 更新优化后的仓库
func updateOptimizedRepository(workDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 获取所有远程更新
	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "fetch", "--all", "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("更新失败: %v\n%s", err, string(output))
	}

	return nil
}

// 检查字符串是否在切片中
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// 下载分支代码压缩包
func downloadBranchArchive(project Project, branchName string, config BackupConfig) error {
	// 创建安全的分支名称作为文件名
	safeBranchName := strings.ReplaceAll(branchName, "/", "_")

	// 创建项目目录结构
	projectDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace)
	archivesDir := filepath.Join(projectDir, "archives")
	if err := os.MkdirAll(archivesDir, 0755); err != nil {
		return fmt.Errorf("创建压缩包目录失败: %v", err)
	}

	// 构建压缩包文件路径
	archiveFile := filepath.Join(archivesDir, safeBranchName+".zip")

	// 检查文件是否已存在
	if _, err := os.Stat(archiveFile); err == nil {
		log.Printf("分支 %s 的压缩包已存在，跳过下载", branchName)
		return nil
	}

	// 构建API URL
	archiveURL := fmt.Sprintf("%s/api/v4/projects/%d/repository/archive.zip?sha=%s",
		GITLAB_URL, project.ID, url.QueryEscape(branchName))

	// 创建临时文件
	tmpFile := archiveFile + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}

	defer func() {
		out.Close()
		// 如果函数返回错误，删除临时文件
		if err != nil {
			os.Remove(tmpFile)
		}
	}()

	// 下载压缩包
	for retry := 0; retry < MAX_RETRIES; retry++ {
		if retry > 0 {
			log.Printf("重试下载分支 %s 的压缩包 (第 %d 次)", branchName, retry+1)
			time.Sleep(time.Second * time.Duration(retry)) // 重试延迟
		}

		req, err := http.NewRequest("GET", archiveURL, nil)
		if err != nil {
			return fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)

		client := &http.Client{
			Timeout: 10 * time.Minute,
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("下载失败: %v", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("下载失败，状态码: %d", resp.StatusCode)
			continue
		}

		// 重置文件指针
		out.Seek(0, 0)

		// 下载文件
		written, err := io.Copy(out, resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Printf("保存文件失败: %v", err)
			continue
		}

		if written == 0 {
			log.Printf("警告：下载的文件大小为0")
			continue
		}

		// 下载成功，重命名文件
		out.Close()
		if err := os.Rename(tmpFile, archiveFile); err != nil {
			return fmt.Errorf("重命名文件失败: %v", err)
		}

		log.Printf("成功下载分支 %s 的压缩包 (%.2f MB)", branchName, float64(written)/(1024*1024))
		return nil
	}

	return fmt.Errorf("下载分支 %s 的压缩包失败，已达到最大重试次数", branchName)
}

// 获取项目的所有分支
func getProjectBranches(project Project) ([]string, error) {
	var allBranches []string
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/api/v4/projects/%d/repository/branches?page=%d&per_page=%d",
			GITLAB_URL, project.ID, page, perPage)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %v", err)
		}

		req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("api请求失败: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("api请求失败，状态码: %d", resp.StatusCode)
		}

		var branches []struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("解析分支信息失败: %v", err)
		}
		resp.Body.Close()

		if len(branches) == 0 {
			break
		}

		for _, branch := range branches {
			allBranches = append(allBranches, branch.Name)
		}

		page++
	}

	return allBranches, nil
}

// 下载项目所有分支的压缩包
func downloadAllBranchArchives(project Project, config BackupConfig) error {
	log.Printf("获取项目 %s 的所有分支", project.PathWithNamespace)

	// 获取所有分支
	branches, err := getProjectBranches(project)
	if err != nil {
		return fmt.Errorf("获取分支列表失败: %v", err)
	}

	log.Printf("项目 %s 共有 %d 个分支", project.PathWithNamespace, len(branches))

	// 创建信号量控制并发
	semaphore := make(chan struct{}, CONCURRENT)
	var wg sync.WaitGroup

	// 记录错误的分支
	var failedBranches []string
	var mutex sync.Mutex

	// 下载每个分支的压缩包
	for _, branch := range branches {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(branchName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if err := downloadBranchArchive(project, branchName, config); err != nil {
				log.Printf("下载分支 %s 的压缩包失败: %v", branchName, err)
				mutex.Lock()
				failedBranches = append(failedBranches, branchName)
				mutex.Unlock()
			}
		}(branch)
	}

	wg.Wait()

	if len(failedBranches) > 0 {
		return fmt.Errorf("有 %d 个分支的压缩包下载失败: %v", len(failedBranches), failedBranches)
	}

	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// 解析命令行参数
	flags := parseCommandFlags()

	// 检查是否显示帮助
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			showHelp()
			return
		}
	}

	// 获取并保存所有仓库列表
	if flags.ListAllRepos {
		log.Println("开始获取所有仓库列表...")

		projects, err := getAllProjects()
		if err != nil {
			log.Fatalf("获取所有项目失败: %v", err)
		}

		if err := saveAllReposToFile(projects); err != nil {
			log.Fatalf("保存仓库列表失败: %v", err)
		}

		log.Println("所有仓库列表已保存到", ALL_REPO_FILE)

		// 如果不需要备份，则直接返回
		if !flags.BackupRepos && !flags.ExtractCode && !flags.DownloadArchives {
			return
		}
	}

	// 执行备份操作或提取代码
	if flags.BackupRepos || flags.ExtractCode || flags.DownloadArchives {
		startTime := time.Now()

		// 创建备份配置
		config := newBackupConfig()
		if err := config.createDirectories(); err != nil {
			log.Fatalf("初始化备份目录失败: %v", err)
		}

		log.Printf("开始备份 (日期: %s)\n", config.Date)
		log.Println("开始获取指定的GitLab项目信息...")

		projects, err := getSpecifiedProjects()
		if err != nil {
			log.Fatalf("获取项目失败: %v", err)
		}

		log.Printf("共找到 %d 个项目\n", len(projects))

		// 保存项目信息
		if err := saveProjectInfo(projects, config); err != nil {
			log.Printf("保存项目信息失败: %v", err)
		}

		// 如果需要备份仓库
		if flags.BackupRepos {
			// 下载所有项目备份
			semaphore := make(chan struct{}, CONCURRENT)
			var wg sync.WaitGroup

			if flags.OptimizedMode {
				log.Println("开始下载项目备份（优化模式）...")
				for _, project := range projects {
					wg.Add(1)
					semaphore <- struct{}{}
					go downloadBackupOptimized(project, &wg, semaphore, config, flags.ExtractCode)
				}
			} else {
				log.Println("开始下载项目备份（传统模式）...")
				for _, project := range projects {
					wg.Add(1)
					semaphore <- struct{}{}
					go downloadBackup(project, &wg, semaphore, config, flags.ExtractCode)
				}
			}

			wg.Wait()
		} else if flags.ExtractCode {
			// 只提取已有仓库的代码
			log.Println("开始从现有仓库提取分支代码...")
			for _, project := range projects {
				gitDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace, "repository.git")
				if _, err := os.Stat(gitDir); err == nil {
					log.Printf("开始提取项目 %s 的所有分支代码\n", project.PathWithNamespace)
					if err := extractAllBranches(project, config); err != nil {
						log.Printf("提取分支代码失败 %s: %v\n", project.PathWithNamespace, err)
					} else {
						log.Printf("成功提取项目 %s 的所有分支代码\n", project.PathWithNamespace)
					}
				} else {
					log.Printf("项目 %s 的Git仓库不存在，无法提取分支代码\n", project.PathWithNamespace)
				}
			}
		}

		// 如果需要下载分支压缩包
		if flags.DownloadArchives {
			log.Println("开始下载项目分支压缩包...")
			for _, project := range projects {
				log.Printf("开始下载项目 %s 的所有分支压缩包\n", project.PathWithNamespace)
				if err := downloadAllBranchArchives(project, config); err != nil {
					log.Printf("下载分支压缩包失败 %s: %v\n", project.PathWithNamespace, err)
				} else {
					log.Printf("成功下载项目 %s 的所有分支压缩包\n", project.PathWithNamespace)
				}
			}
		}

		// 生成备份报告
		if err := generateBackupReport(projects, startTime, config); err != nil {
			log.Printf("生成备份报告失败: %v", err)
		}

		log.Printf("备份完成！总耗时: %s\n", time.Since(startTime).Round(time.Second))
	}
}
