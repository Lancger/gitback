package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	GITLAB_URL    = "https://git.qq.com"   // 替换为您的 GitLab 实例地址
	PRIVATE_TOKEN = "x7TfeZy49Ks3LT4Hx9bw" // 替换为您的私人令牌
	MAX_RETRIES   = 3                      // 最大重试次数
	CONCURRENT    = 5                      // 并发下载数
	REPO_FILE     = "repo.txt"             // 存储仓库URL的文件
	ALL_REPO_FILE = "all_repos.txt"        // 存储所有仓库URL的文件
)

// 内置的默认仓库列表
var defaultRepos = []string{
	"#项目",
	"https://git.qq.com/aa/aabb-mng-web.git",
	"https://git.qq.com/aa/aabb-web.git",
	"https://git.qq.com/aa/aabb-app.git",
	"https://git.qq.com/aa/aabb-mng.git",
	"https://git.qq.com/aa/phone-msg.git",
	"https://git.qq.com/aa/swap-master.git",
	"https://git.qq.com/aa/x-swap.git",
	"https://git.qq.com/aa/swap-ui.git",
	"https://git.qq.com/aa/aabb-h5.git",
	"https://git.qq.com/aa/app-code-editing.git",
	"",
	"#项目",
	"https://git.qq.com/bb_backend/aabb-mng-big.git",
	"https://git.qq.com/bb_frontend/bb-web.git",
	"https://git.qq.com/bb_frontend/bb-admin-manager.git",
	"https://git.qq.com/bb_frontend/bb-app.git",
}

// 命令行参数
type CommandFlags struct {
	ListAllRepos bool // 是否获取并保存所有仓库列表
	BackupRepos  bool // 是否备份仓库
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
			return nil, fmt.Errorf("API请求失败: %d", resp.StatusCode)
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
		return Project{}, fmt.Errorf("API请求失败: %d", resp.StatusCode)
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
		return Project{}, fmt.Errorf("API搜索请求失败: %d", resp.StatusCode)
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
		return Project{}, fmt.Errorf("API请求失败: %d", resp.StatusCode)
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
		return Project{}, fmt.Errorf("API请求失败: %d", resp.StatusCode)
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

func downloadBackup(project Project, wg *sync.WaitGroup, semaphore chan struct{}, config BackupConfig) {
	defer wg.Done()
	defer func() { <-semaphore }()

	backupURL := fmt.Sprintf("%s/api/v4/projects/%d/repository/archive.zip", GITLAB_URL, project.ID)
	projectDir := filepath.Join(config.ProjectsDir, project.PathWithNamespace)
	fileName := filepath.Join(projectDir, "repository.zip")

	// 创建项目目录
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		log.Printf("创建目录失败 %s: %v\n", projectDir, err)
		return
	}

	// 检查文件是否已存在
	if _, err := os.Stat(fileName); err == nil {
		log.Printf("文件已存在，跳过下载 %s\n", fileName)
		return
	}

	// 创建临时文件
	tmpFile := fileName + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		log.Printf("创建临时文件失败 %s: %v\n", tmpFile, err)
		return
	}

	success := false
	defer func() {
		out.Close()
		if !success {
			os.Remove(tmpFile) // 如果下载失败，删除临时文件
		}
	}()

	// 下载文件
	for retry := 0; retry < MAX_RETRIES; retry++ {
		if retry > 0 {
			log.Printf("重试下载 %s (第 %d 次)\n", project.PathWithNamespace, retry+1)
			time.Sleep(time.Second * time.Duration(retry)) // 重试延迟
		}

		req, err := http.NewRequest("GET", backupURL, nil)
		if err != nil {
			log.Printf("创建请求失败 %s: %v\n", backupURL, err)
			continue
		}

		req.Header.Set("PRIVATE-TOKEN", PRIVATE_TOKEN)
		client := &http.Client{
			Timeout: 30 * time.Minute, // 设置较长的超时时间
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("下载失败 %s: %v\n", backupURL, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("下载失败 %s: 状态码 %d\n", backupURL, resp.StatusCode)
			continue
		}

		// 重置文件指针到开始位置
		out.Seek(0, 0)

		// 使用io.Copy进行下载，并显示进度
		written, err := io.Copy(out, resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Printf("保存文件失败 %s: %v\n", fileName, err)
			continue
		}

		if written == 0 {
			log.Printf("警告：下载的文件大小为0 %s\n", project.PathWithNamespace)
			continue
		}

		// 下载成功，将临时文件重命名为最终文件
		out.Close()
		if err := os.Rename(tmpFile, fileName); err != nil {
			log.Printf("重命名文件失败 %s: %v\n", fileName, err)
			return
		}

		success = true
		log.Printf("成功下载项目 %s (%.2f MB)\n", project.PathWithNamespace, float64(written)/(1024*1024))
		return
	}

	log.Printf("下载失败，已达到最大重试次数 %s\n", project.PathWithNamespace)
}

func generateBackupReport(projects []Project, startTime time.Time, config BackupConfig) error {
	endTime := time.Now()
	duration := endTime.Sub(startTime)

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
	fmt.Fprintf(f, "备份目录: %s\n", config.BackupDir)
	fmt.Fprintf(f, "%s\n", strings.Repeat("=", 50))

	return nil
}

// 解析命令行参数
func parseCommandFlags() CommandFlags {
	flags := CommandFlags{
		ListAllRepos: false,
		BackupRepos:  true, // 默认执行备份操作
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
		}
	}

	return flags
}

// 显示使用帮助
func showHelp() {
	fmt.Println("GitLab仓库备份工具")
	fmt.Println("用法:")
	fmt.Println("  无参数     - 默认备份repo.txt中指定的仓库")
	fmt.Println("  -l, --list - 获取所有仓库列表并保存到all_repos.txt")
	fmt.Println("  -b, --backup - 备份repo.txt中指定的仓库")
	fmt.Println("  -a, --all   - 获取所有仓库列表并备份repo.txt中的仓库")
	fmt.Println("  -h, --help  - 显示此帮助信息")
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
		if !flags.BackupRepos {
			return
		}
	}

	// 执行备份操作
	if flags.BackupRepos {
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

		// 下载所有项目备份
		semaphore := make(chan struct{}, CONCURRENT)
		var wg sync.WaitGroup

		log.Println("开始下载项目备份...")
		for _, project := range projects {
			wg.Add(1)
			semaphore <- struct{}{}
			go downloadBackup(project, &wg, semaphore, config)
		}

		wg.Wait()

		// 生成备份报告
		if err := generateBackupReport(projects, startTime, config); err != nil {
			log.Printf("生成备份报告失败: %v", err)
		}

		log.Printf("备份完成！总耗时: %s\n", time.Since(startTime).Round(time.Second))
	}
}
